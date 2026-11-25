package token

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/oklog/ulid/v2"
)

func decodeSegment(t *testing.T, segment string) map[string]any {
	t.Helper()
	data, err := base64.RawURLEncoding.DecodeString(segment)
	if err != nil {
		t.Fatalf("decode segment: %v", err)
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	out := map[string]any{}
	if err := dec.Decode(&out); err != nil {
		t.Fatalf("unmarshal segment: %v", err)
	}
	return out
}

func issueParts(t *testing.T, lifetime time.Duration) (map[string]any, map[string]any) {
	t.Helper()
	raw, err := Issue(testKey, "agent-a", "Agent A", testNow, lifetime)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		t.Fatalf("Issue returned %d parts; want 3", len(parts))
	}
	return decodeSegment(t, parts[0]), decodeSegment(t, parts[1])
}

func TestClosedClaims(t *testing.T) {
	header, claims := issueParts(t, time.Hour)
	if header["alg"] != "HS256" {
		t.Fatalf("alg = %v; want HS256", header["alg"])
	}
	var names []string
	for name := range claims {
		names = append(names, name)
	}
	slices.Sort(names)
	want := []string{"exp", "iat", "jti", "kind", "name", "sub"}
	if !slices.Equal(names, want) {
		t.Fatalf("claims = %v; want %v", names, want)
	}
	if claims["sub"] != "agent-a" || claims["name"] != "Agent A" || claims["kind"] != Kind {
		t.Fatalf("claims = %v", claims)
	}
	jti, _ := claims["jti"].(string)
	id, err := ulid.ParseStrict(jti)
	if err != nil {
		t.Fatalf("jti %q is not a ULID: %v", jti, err)
	}
	if ulid.Time(id.Time()).Unix() != testNow.Unix() {
		t.Fatalf("jti time = %v; want %v", ulid.Time(id.Time()), testNow)
	}
}

func TestExpEqualsIatPlusLifetime(t *testing.T) {
	lifetime := 8760 * time.Hour
	_, claims := issueParts(t, lifetime)
	iat, err := claims["iat"].(json.Number).Int64()
	if err != nil {
		t.Fatalf("iat is not an integer: %v", claims["iat"])
	}
	exp, err := claims["exp"].(json.Number).Int64()
	if err != nil {
		t.Fatalf("exp is not an integer: %v", claims["exp"])
	}
	if iat != testNow.Unix() {
		t.Fatalf("iat = %d; want %d", iat, testNow.Unix())
	}
	if exp != iat+int64(lifetime/time.Second) {
		t.Fatalf("exp = %d; want iat + %d", exp, int64(lifetime/time.Second))
	}
}

func TestIssueVerifyRoundTrip(t *testing.T) {
	raw, err := Issue(testKey, "agent-a", "Agent A", testNow, time.Hour)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	got, err := Verify(testKey, raw, testNow)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if got.Sub != "agent-a" || got.Name != "Agent A" || got.Kind != Kind {
		t.Fatalf("Verify = %+v", got)
	}
	if !got.ExpiresAt.Equal(testNow.Add(time.Hour)) || !got.IssuedAt.Equal(testNow) {
		t.Fatalf("times = %v, %v", got.IssuedAt, got.ExpiresAt)
	}
}

func TestIssueUniqueJTI(t *testing.T) {
	a, _ := Issue(testKey, "agent-a", "Agent A", testNow, time.Hour)
	b, _ := Issue(testKey, "agent-a", "Agent A", testNow, time.Hour)
	if a == b {
		t.Fatal("two tokens are equal")
	}
}

func TestIssueRejectsInvalidInput(t *testing.T) {
	cases := []struct {
		name     string
		key      []byte
		sub      string
		display  string
		lifetime time.Duration
	}{
		{"empty key", nil, "agent-a", "A", time.Hour},
		{"short key", bytes.Repeat([]byte{1}, 31), "agent-a", "A", time.Hour},
		{"sub uppercase", testKey, "Agent", "A", time.Hour},
		{"sub empty", testKey, "", "A", time.Hour},
		{"name empty", testKey, "agent-a", "", time.Hour},
		{"name blank", testKey, "agent-a", "  ", time.Hour},
		{"name too long", testKey, "agent-a", strings.Repeat("n", 65), time.Hour},
		{"zero lifetime", testKey, "agent-a", "A", 0},
		{"sub-second lifetime", testKey, "agent-a", "A", 500 * time.Millisecond},
		{"negative lifetime", testKey, "agent-a", "A", -time.Hour},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := Issue(tc.key, tc.sub, tc.display, testNow, tc.lifetime)
			if err == nil || raw != "" {
				t.Fatalf("Issue = %q, %v; want an error and no token", raw, err)
			}
		})
	}
}
