package token

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var (
	testKey   = bytes.Repeat([]byte{0x11}, 32)
	otherKey  = bytes.Repeat([]byte{0x22}, 32)
	testNow   = time.Unix(1_800_000_000, 0)
	testJTI   = "01JDQZ3V9X8K4M2N6P7R8S9T0V"
	hourInSec = int64(3600)
)

func validClaims() jwt.MapClaims {
	return jwt.MapClaims{
		"sub":  "agent-a",
		"name": "Agent A",
		"kind": "agent",
		"iat":  testNow.Unix() - hourInSec,
		"exp":  testNow.Unix() + hourInSec,
		"jti":  testJTI,
	}
}

func with(c jwt.MapClaims, key string, value any) jwt.MapClaims {
	out := jwt.MapClaims{}
	for k, v := range c {
		out[k] = v
	}
	out[key] = value
	return out
}

func without(c jwt.MapClaims, key string) jwt.MapClaims {
	out := jwt.MapClaims{}
	for k, v := range c {
		if k != key {
			out[k] = v
		}
	}
	return out
}

func sign(t *testing.T, method jwt.SigningMethod, key any, c jwt.MapClaims) string {
	t.Helper()
	raw, err := jwt.NewWithClaims(method, c).SignedString(key)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	return raw
}

func signHS256(t *testing.T, c jwt.MapClaims) string {
	return sign(t, jwt.SigningMethodHS256, testKey, c)
}

func unsignedNone(t *testing.T, c jwt.MapClaims) string {
	t.Helper()
	header, _ := json.Marshal(map[string]string{"alg": "none", "typ": "JWT"})
	payload, err := json.Marshal(c)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	enc := base64.RawURLEncoding
	return enc.EncodeToString(header) + "." + enc.EncodeToString(payload) + "."
}

func assertCause(t *testing.T, raw string, key []byte, want Cause) {
	t.Helper()
	claims, err := Verify(key, raw, testNow)
	var tokenErr *Error
	if !errors.As(err, &tokenErr) {
		t.Fatalf("Verify error = %v; want *token.Error with cause %q", err, want)
	}
	if tokenErr.Cause != want {
		t.Fatalf("Verify cause = %q; want %q", tokenErr.Cause, want)
	}
	if claims != (Claims{}) {
		t.Fatalf("Verify returned claims %+v on failure", claims)
	}
}

func TestAC03_Rejects(t *testing.T) {
	cases := []struct {
		name string
		raw  func(t *testing.T) string
		want Cause
	}{
		{"wrong signature", func(t *testing.T) string {
			return sign(t, jwt.SigningMethodHS256, otherKey, validClaims())
		}, CauseUnauthenticated},
		{"alg none", func(t *testing.T) string {
			return unsignedNone(t, validClaims())
		}, CauseUnauthenticated},
		{"alg HS512", func(t *testing.T) string {
			return sign(t, jwt.SigningMethodHS512, testKey, validClaims())
		}, CauseUnauthenticated},
		{"unknown claim", func(t *testing.T) string {
			return signHS256(t, with(validClaims(), "role", "admin"))
		}, CauseUnknownClaim},
		{"missing jti", func(t *testing.T) string {
			return signHS256(t, without(validClaims(), "jti"))
		}, CauseMissingClaim},
		{"kind other than agent", func(t *testing.T) string {
			return signHS256(t, with(validClaims(), "kind", "human"))
		}, CauseInvalidType},
		{"expired", func(t *testing.T) string {
			c := with(validClaims(), "iat", testNow.Unix()-2*hourInSec)
			return signHS256(t, with(c, "exp", testNow.Unix()-hourInSec))
		}, CauseExpired},
		{"iat in the future", func(t *testing.T) string {
			c := with(validClaims(), "iat", testNow.Unix()+hourInSec)
			return signHS256(t, with(c, "exp", testNow.Unix()+2*hourInSec))
		}, CauseNotYetIssued},
		{"auth.version change", func(t *testing.T) string {
			return sign(t, jwt.SigningMethodHS256, otherKey, validClaims())
		}, CauseUnauthenticated},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertCause(t, tc.raw(t), testKey, tc.want)
		})
	}
}

func TestVerifyAcceptsValid(t *testing.T) {
	got, err := Verify(testKey, signHS256(t, validClaims()), testNow)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	want := Claims{
		Sub:       "agent-a",
		Name:      "Agent A",
		Kind:      Kind,
		IssuedAt:  time.Unix(testNow.Unix()-hourInSec, 0),
		ExpiresAt: time.Unix(testNow.Unix()+hourInSec, 0),
		JTI:       testJTI,
	}
	if !got.IssuedAt.Equal(want.IssuedAt) || !got.ExpiresAt.Equal(want.ExpiresAt) {
		t.Fatalf("times = %v, %v; want %v, %v", got.IssuedAt, got.ExpiresAt, want.IssuedAt, want.ExpiresAt)
	}
	got.IssuedAt, got.ExpiresAt, want.IssuedAt, want.ExpiresAt = time.Time{}, time.Time{}, time.Time{}, time.Time{}
	if got != want {
		t.Fatalf("Verify = %+v; want %+v", got, want)
	}
}

func TestVerifyOrder(t *testing.T) {
	expiredFuture := with(with(validClaims(), "exp", testNow.Unix()), "iat", testNow.Unix()+hourInSec)
	cases := []struct {
		name string
		raw  func(t *testing.T) string
		want Cause
	}{
		{"signature before unknown claim", func(t *testing.T) string {
			return sign(t, jwt.SigningMethodHS256, otherKey, with(validClaims(), "role", "admin"))
		}, CauseUnauthenticated},
		{"unknown claim before invalid type", func(t *testing.T) string {
			return signHS256(t, with(with(validClaims(), "role", "admin"), "sub", 7))
		}, CauseUnknownClaim},
		{"unknown claim before missing claim", func(t *testing.T) string {
			return signHS256(t, with(without(validClaims(), "jti"), "role", "admin"))
		}, CauseUnknownClaim},
		{"invalid type before missing claim", func(t *testing.T) string {
			return signHS256(t, with(without(validClaims(), "jti"), "kind", "human"))
		}, CauseInvalidType},
		{"missing claim before expired", func(t *testing.T) string {
			return signHS256(t, with(without(validClaims(), "jti"), "exp", testNow.Unix()-hourInSec))
		}, CauseMissingClaim},
		{"invalid type before expired", func(t *testing.T) string {
			return signHS256(t, with(with(validClaims(), "name", ""), "exp", testNow.Unix()-hourInSec))
		}, CauseInvalidType},
		{"expired before not yet issued", func(t *testing.T) string {
			return signHS256(t, expiredFuture)
		}, CauseExpired},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertCause(t, tc.raw(t), testKey, tc.want)
		})
	}
}

func TestVerifyInvalidType(t *testing.T) {
	cases := map[string]jwt.MapClaims{
		"sub not string":       with(validClaims(), "sub", 1),
		"sub uppercase":        with(validClaims(), "sub", "Agent"),
		"sub leading digit":    with(validClaims(), "sub", "1agent"),
		"sub too long":         with(validClaims(), "sub", "a"+string(bytes.Repeat([]byte("b"), 63))),
		"sub empty":            with(validClaims(), "sub", ""),
		"name not string":      with(validClaims(), "name", true),
		"name empty":           with(validClaims(), "name", ""),
		"name blank":           with(validClaims(), "name", "   "),
		"name too long":        with(validClaims(), "name", string(bytes.Repeat([]byte("n"), 65))),
		"kind not string":      with(validClaims(), "kind", 1),
		"kind empty":           with(validClaims(), "kind", ""),
		"iat string":           with(validClaims(), "iat", "1"),
		"iat fraction":         with(validClaims(), "iat", 1.5),
		"exp string":           with(validClaims(), "exp", "1"),
		"exp fraction":         with(validClaims(), "exp", float64(testNow.Unix())+0.5),
		"exp null":             with(validClaims(), "exp", nil),
		"jti not ulid":         with(validClaims(), "jti", "not-a-ulid"),
		"jti not string":       with(validClaims(), "jti", 5),
		"jti overflows ulid":   with(validClaims(), "jti", "81JDQZ3V9X8K4M2N6P7R8S9T0V"),
		"sub with whitespace":  with(validClaims(), "sub", "agent a"),
		"name list":            with(validClaims(), "name", []string{"a"}),
		"iat exponent float64": with(validClaims(), "iat", 1e300),
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			assertCause(t, signHS256(t, c), testKey, CauseInvalidType)
		})
	}
}

func TestVerifyAcceptsBoundaryForms(t *testing.T) {
	cases := map[string]jwt.MapClaims{
		"sub 63 chars":    with(validClaims(), "sub", "a"+string(bytes.Repeat([]byte("b"), 62))),
		"name 64 runes":   with(validClaims(), "name", string(bytes.Repeat([]byte("é"), 32))+string(bytes.Repeat([]byte("n"), 32))),
		"iat equals now":  with(validClaims(), "iat", testNow.Unix()),
		"sub with hyphen": with(validClaims(), "sub", "a-1"),
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Verify(testKey, signHS256(t, c), testNow); err != nil {
				t.Fatalf("Verify: %v", err)
			}
		})
	}
}

func TestVerifyMissingEachClaim(t *testing.T) {
	for _, name := range []string{"sub", "name", "kind", "iat", "exp", "jti"} {
		t.Run(name, func(t *testing.T) {
			assertCause(t, signHS256(t, without(validClaims(), name)), testKey, CauseMissingClaim)
		})
	}
}

func TestVerifyExpEqualsNowIsExpired(t *testing.T) {
	assertCause(t, signHS256(t, with(validClaims(), "exp", testNow.Unix())), testKey, CauseExpired)
}

func TestVerifyRejectsMalformed(t *testing.T) {
	valid := signHS256(t, validClaims())
	cases := map[string]string{
		"empty":         "",
		"two parts":     "a.b",
		"garbage":       "not a token",
		"trailing part": valid + ".x",
		"payload array": func() string {
			enc := base64.RawURLEncoding
			header := enc.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
			payload := enc.EncodeToString([]byte(`[1,2]`))
			s, _ := jwt.SigningMethodHS256.Sign(header+"."+payload, testKey)
			return header + "." + payload + "." + enc.EncodeToString(s)
		}(),
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			assertCause(t, raw, testKey, CauseUnauthenticated)
		})
	}
}

func TestVerifyRejectsWeakKey(t *testing.T) {
	for _, key := range [][]byte{nil, {}, bytes.Repeat([]byte{1}, 31)} {
		raw := sign(t, jwt.SigningMethodHS256, key, validClaims())
		assertCause(t, raw, key, CauseUnauthenticated)
	}
}

func TestErrorTextHasNoToken(t *testing.T) {
	raw := sign(t, jwt.SigningMethodHS256, otherKey, validClaims())
	_, err := Verify(testKey, raw, testNow)
	if err == nil {
		t.Fatal("Verify accepted a token with a wrong signature")
	}
	if bytes.Contains([]byte(err.Error()), []byte(raw[:20])) {
		t.Fatalf("error text %q holds the token", err.Error())
	}
}
