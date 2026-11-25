//go:build e2e

package e2e

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/oklog/ulid/v2"

	"github.com/kanthorlabs/sag/e2e/harness"
	"github.com/kanthorlabs/sag/internal/keys"
	"github.com/kanthorlabs/sag/internal/token"
)

const ac03Sub = "ac03"

const ac03Initialize = `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"ac03","version":"1"}}}`

func init() {
	harness.Register(harness.AC{
		ID:     "AC03",
		Phase:  harness.PhaseParallel,
		Agents: []harness.AgentSpec{{Sub: ac03Sub, Name: "AC03"}},
		Resources: []harness.ResourceSpec{{
			Name:       "ac03",
			Containers: []string{harness.ContainerSAG, harness.ContainerV2},
			Agents:     []string{ac03Sub},
		}},
		Run: runAC03,
	})
}

func ac03Claims(now time.Time) jwt.MapClaims {
	return jwt.MapClaims{
		"sub":  ac03Sub,
		"name": "AC03",
		"kind": token.Kind,
		"iat":  now.Add(-time.Hour).Unix(),
		"exp":  now.Add(time.Hour).Unix(),
		"jti":  ulid.Make().String(),
	}
}

func ac03With(c jwt.MapClaims, name string, value any) jwt.MapClaims {
	out := jwt.MapClaims{}
	for k, v := range c {
		out[k] = v
	}
	if value == nil {
		delete(out, name)
	} else {
		out[name] = value
	}
	return out
}

func ac03Sign(t *testing.T, method jwt.SigningMethod, key []byte, c jwt.MapClaims) string {
	t.Helper()
	raw, err := jwt.NewWithClaims(method, c).SignedString(key)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	return raw
}

func ac03None(t *testing.T, c jwt.MapClaims) string {
	t.Helper()
	payload, err := json.Marshal(c)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	enc := base64.RawURLEncoding
	return enc.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`)) + "." + enc.EncodeToString(payload) + "."
}

func ac03Key(t *testing.T, env harness.Env, version int) []byte {
	t.Helper()
	key, err := keys.JWTKey(env.MasterKey(), version)
	if err != nil {
		t.Fatalf("derive JWT key v%d: %v", version, err)
	}
	return key
}

func ac03MCPURL(env harness.Env, container string) string {
	u := strings.TrimSuffix(env.URL(container), "/")
	if strings.HasSuffix(u, "/mcp") {
		return u
	}
	return u + "/mcp"
}

func ac03Post(ctx context.Context, t *testing.T, env harness.Env, container, raw string) *http.Response {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ac03MCPURL(env, container), strings.NewReader(ac03Initialize))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Authorization", "Bearer "+raw)
	resp, err := env.HTTPClient().Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", container, err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

func ac03ExpectAccepted(ctx context.Context, t *testing.T, env harness.Env, container, raw string) {
	t.Helper()
	resp := ac03Post(ctx, t, env, container, raw)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("%s: valid JWT got HTTP %d; want 200", container, resp.StatusCode)
	}
}

func ac03ExpectRejected(ctx context.Context, t *testing.T, env harness.Env, container, raw string, cause token.Cause) {
	t.Helper()
	resp := ac03Post(ctx, t, env, container, raw)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("%s: HTTP %d; want 401", container, resp.StatusCode)
	}
	challenge := resp.Header.Get("WWW-Authenticate")
	if !strings.HasPrefix(challenge, "Bearer") {
		t.Fatalf("%s: WWW-Authenticate = %q; want Bearer", container, challenge)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if cause != token.CauseUnauthenticated && strings.Contains(string(body)+challenge, string(cause)) {
		t.Fatalf("%s: response reveals the cause %q", container, cause)
	}
}

func runAC03(t *testing.T, env harness.Env) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	now := time.Now()
	v1 := ac03Key(t, env, 1)
	v2 := ac03Key(t, env, 2)
	otherKey := make([]byte, 32)
	if _, err := rand.Read(otherKey); err != nil {
		t.Fatalf("random key: %v", err)
	}

	t.Run("controls", func(t *testing.T) {
		ac03ExpectAccepted(ctx, t, env, harness.ContainerSAG, ac03Sign(t, jwt.SigningMethodHS256, v1, ac03Claims(now)))
		ac03ExpectAccepted(ctx, t, env, harness.ContainerV2, ac03Sign(t, jwt.SigningMethodHS256, v2, ac03Claims(now)))

		issuedV1, err := env.Token(ctx, harness.ContainerSAG, ac03Sub)
		if err != nil {
			t.Fatalf("jwt generate on %s: %v", harness.ContainerSAG, err)
		}
		ac03ExpectAccepted(ctx, t, env, harness.ContainerSAG, issuedV1)
		ac03ExpectRejected(ctx, t, env, harness.ContainerV2, issuedV1, token.CauseUnauthenticated)

		issuedV2, err := env.Token(ctx, harness.ContainerV2, ac03Sub)
		if err != nil {
			t.Fatalf("jwt generate on %s: %v", harness.ContainerV2, err)
		}
		ac03ExpectAccepted(ctx, t, env, harness.ContainerV2, issuedV2)
		ac03ExpectRejected(ctx, t, env, harness.ContainerSAG, issuedV2, token.CauseUnauthenticated)
	})

	valid := ac03Claims(now)
	cases := []struct {
		name      string
		container string
		raw       string
		cause     token.Cause
	}{
		{"wrong signature", harness.ContainerSAG, ac03Sign(t, jwt.SigningMethodHS256, otherKey, valid), token.CauseUnauthenticated},
		{"alg none", harness.ContainerSAG, ac03None(t, valid), token.CauseUnauthenticated},
		{"alg HS512", harness.ContainerSAG, ac03Sign(t, jwt.SigningMethodHS512, v1, valid), token.CauseUnauthenticated},
		{"unknown claim", harness.ContainerSAG, ac03Sign(t, jwt.SigningMethodHS256, v1, ac03With(valid, "role", "admin")), token.CauseUnknownClaim},
		{"missing jti", harness.ContainerSAG, ac03Sign(t, jwt.SigningMethodHS256, v1, ac03With(valid, "jti", nil)), token.CauseMissingClaim},
		{"kind other than agent", harness.ContainerSAG, ac03Sign(t, jwt.SigningMethodHS256, v1, ac03With(valid, "kind", "human")), token.CauseInvalidType},
		{"expired", harness.ContainerSAG, ac03Sign(t, jwt.SigningMethodHS256, v1, ac03With(ac03With(valid, "iat", now.Add(-2*time.Hour).Unix()), "exp", now.Add(-time.Hour).Unix())), token.CauseExpired},
		{"iat in the future", harness.ContainerSAG, ac03Sign(t, jwt.SigningMethodHS256, v1, ac03With(ac03With(valid, "iat", now.Add(time.Hour).Unix()), "exp", now.Add(2*time.Hour).Unix())), token.CauseNotYetIssued},
		{"auth.version 1 after change to 2", harness.ContainerV2, ac03Sign(t, jwt.SigningMethodHS256, v1, valid), token.CauseUnauthenticated},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ac03ExpectRejected(ctx, t, env, tc.container, tc.raw, tc.cause)
		})
	}
}
