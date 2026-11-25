//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/kanthorlabs/sag/e2e/harness"
)

const (
	ac28Sub          = "ac28"
	ac28Container    = "sag-ac28"
	ac28Allowed      = "https://agent.example"
	ac28Evil         = "https://evil.example"
	ac28Initialize   = `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"ac28","version":"1"}}}`
	ac28CauseOrigin  = "origin"
	ac28CauseMissing = "missing_bearer"
)

var (
	ac28Agents    = []harness.AgentSpec{{Sub: ac28Sub, Name: "AC28"}}
	ac28Resources = []harness.ResourceSpec{{Name: "ac28", Agents: []string{ac28Sub}}}
)

func init() {
	harness.Register(harness.AC{
		ID:        "AC28",
		Phase:     harness.PhaseParallel,
		Agents:    ac28Agents,
		Resources: ac28Resources,
		Run:       runAC28,
	})
}

func runAC28(t *testing.T, env harness.Env) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	ac28Start(ctx, t, env)
	jwt, err := env.Token(ctx, ac28Container, ac28Sub)
	if err != nil {
		t.Fatalf("token: %v", err)
	}

	resp := ac28Post(ctx, t, env, "", "")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("no JWT: HTTP %d; want 401", resp.StatusCode)
	}
	ac28ExpectForbidden(ctx, t, env, "", ac28Evil)
	ac28ExpectForbidden(ctx, t, env, jwt, ac28Evil)

	resp = ac28Post(ctx, t, env, "", ac28Allowed)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("allowed origin without JWT: HTTP %d; want 401", resp.StatusCode)
	}
	resp = ac28Post(ctx, t, env, jwt, ac28Allowed)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("allowed origin with JWT: HTTP %d; want 200", resp.StatusCode)
	}

	causes := ac28AuthCauses(ctx, t, env)
	want := []string{ac28CauseMissing, ac28CauseOrigin, ac28CauseOrigin, ac28CauseMissing}
	if strings.Join(causes, ",") != strings.Join(want, ",") {
		t.Fatalf("authentication failure causes = %q; want %q (one origin record per 403)", causes, want)
	}
}

func ac28Start(ctx context.Context, t *testing.T, env harness.Env) {
	t.Helper()
	config, err := harness.Compose([]harness.AC{{ID: "AC28", Agents: ac28Agents, Resources: ac28Resources}}, harness.ContainerSAG)
	if err != nil {
		t.Fatalf("compose: %v", err)
	}
	config = strings.Replace(config, "server:\n", "server:\n    allowed_origins: [\""+ac28Allowed+"\"]\n", 1)
	c, err := env.StartSAG(ctx, harness.ContainerSpec{Name: ac28Container, Config: config})
	if err != nil {
		t.Fatalf("start %s: %v", ac28Container, err)
	}
	t.Cleanup(func() { _ = c.Remove(context.Background()) })

	deadline := time.Now().Add(2 * time.Minute)
	for {
		probe, cancelProbe := context.WithTimeout(ctx, 5*time.Second)
		err := c.Healthz(probe)
		cancelProbe()
		if err == nil {
			return
		}
		if time.Now().After(deadline) {
			logs, _ := c.Logs(ctx)
			t.Fatalf("%s not healthy: %v\nconfig:\n%s\nlogs:\n%s", ac28Container, err, config, logs)
		}
		time.Sleep(time.Second)
	}
}

func ac28Post(ctx context.Context, t *testing.T, env harness.Env, jwt, origin string) *http.Response {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, env.URL(ac28Container)+"/mcp", strings.NewReader(ac28Initialize))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if jwt != "" {
		req.Header.Set("Authorization", "Bearer "+jwt)
	}
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	resp, err := env.HTTPClient().Do(req)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

func ac28ExpectForbidden(ctx context.Context, t *testing.T, env harness.Env, jwt, origin string) {
	t.Helper()
	resp := ac28Post(ctx, t, env, jwt, origin)
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("Origin %s, JWT %v: HTTP %d; want 403", origin, jwt != "", resp.StatusCode)
	}
	if len(body) != 0 || resp.Header.Get("WWW-Authenticate") != "" {
		t.Fatalf("403 body %q, WWW-Authenticate %q; want both empty", body, resp.Header.Get("WWW-Authenticate"))
	}
}

func ac28AuthCauses(ctx context.Context, t *testing.T, env harness.Env) []string {
	t.Helper()
	records, err := env.Audit(ctx, ac28Container)
	if err != nil {
		t.Fatalf("audit: %v", err)
	}
	var causes []string
	for _, raw := range records {
		var rec map[string]any
		if err := json.Unmarshal(raw, &rec); err != nil {
			t.Fatalf("audit line %s: %v", raw, err)
		}
		if rec["sub"] != nil {
			t.Fatalf("audit record with sub; want authentication failures only: %s", raw)
		}
		if rec["jti"] != nil || rec["decision"] != "deny" || rec["reason"] != "unauthenticated" {
			t.Fatalf("authentication failure record = %s", raw)
		}
		cause, _ := rec["auth_cause"].(string)
		causes = append(causes, cause)
	}
	return causes
}
