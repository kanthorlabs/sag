//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/kanthorlabs/sag/e2e/harness"
)

const (
	ac24Sub      = "ac24"
	ac24Resource = "badpass"
	ac24Message  = "resource unavailable"
)

func init() {
	harness.Register(harness.AC{
		ID:     "AC24",
		Phase:  harness.PhaseParallel,
		Agents: []harness.AgentSpec{{Sub: ac24Sub, Name: "AC24"}},
		Resources: []harness.ResourceSpec{{
			Name:          ac24Resource,
			Containers:    []string{harness.ContainerSAG},
			WrongPassword: true,
			Agents:        []string{ac24Sub},
		}},
		Run: runAC24,
	})
}

type ac24ToolError struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id"`
}

type ac24AuditRecord struct {
	RequestID string `json:"request_id"`
	Resource  string `json:"resource"`
	Decision  string `json:"decision"`
	Reason    string `json:"reason"`
}

func ac24Secrets(ctx context.Context, t *testing.T, env harness.Env) []string {
	t.Helper()
	dsn, err := harness.ReadContainerFile(ctx, env.ContainerName(harness.ContainerSAG), "/run/secrets/"+harness.ConnectionSecret(ac24Resource))
	if err != nil {
		t.Fatalf("read connection secret: %v", err)
	}
	u, err := url.Parse(strings.TrimSpace(string(dsn)))
	if err != nil {
		t.Fatal("connection secret does not parse as a URL")
	}
	password, ok := u.User.Password()
	if !ok || len(password) < 16 {
		t.Fatal("connection secret has no wrong password")
	}
	return []string{password, strings.TrimSpace(string(dsn))}
}

func ac24AssertClean(t *testing.T, where, text string, secrets []string) {
	t.Helper()
	for _, s := range secrets {
		if strings.Contains(text, s) {
			t.Fatalf("%s holds the connection secret or its password", where)
		}
	}
}

func ac24WaitPreflightFailed(ctx context.Context, t *testing.T, env harness.Env) string {
	t.Helper()
	deadline := time.Now().Add(70 * time.Second)
	for {
		logs, err := env.Logs(ctx, harness.ContainerSAG)
		if err != nil {
			t.Fatalf("logs: %v", err)
		}
		for _, line := range strings.Split(logs, "\n") {
			if strings.Contains(line, `"resource":"`+ac24Resource+`"`) && strings.Contains(line, `"level":"ERROR"`) {
				return logs
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("no failure log line for resource %s", ac24Resource)
		}
		time.Sleep(2 * time.Second)
	}
}

func ac24AssertAuditRecord(ctx context.Context, t *testing.T, env harness.Env, requestID string, secrets []string) {
	t.Helper()
	lines, err := env.Audit(ctx, harness.ContainerSAG)
	if err != nil {
		t.Fatalf("audit: %v", err)
	}
	found := false
	for _, line := range lines {
		ac24AssertClean(t, "audit copy", string(line), secrets)
		var rec ac24AuditRecord
		if err := json.Unmarshal(line, &rec); err != nil {
			t.Fatalf("audit line does not decode: %v", err)
		}
		if rec.RequestID == requestID {
			found = true
			if rec.Resource != ac24Resource || rec.Decision != "deny" || rec.Reason != "resource_unavailable" {
				t.Fatalf("audit record %+v; want resource %s, decision deny, reason resource_unavailable", rec, ac24Resource)
			}
		}
	}
	if !found {
		t.Fatalf("audit copy has no record for request_id %s", requestID)
	}
}

func runAC24(t *testing.T, env harness.Env) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	secrets := ac24Secrets(ctx, t, env)
	ac24WaitPreflightFailed(ctx, t, env)

	session, err := env.Client(ctx, harness.ContainerSAG, ac24Sub)
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	defer session.Close()

	res := conformanceQuery(ctx, t, session, ac24Resource, "SELECT 1")
	text := conformanceText(res)
	ac24AssertClean(t, "response", text, secrets)
	var te ac24ToolError
	if !res.IsError {
		t.Fatalf("sql_query on %s: no error result: %s", ac24Resource, text)
	}
	if err := json.Unmarshal([]byte(text), &te); err != nil {
		t.Fatalf("error text %q: %v", text, err)
	}
	if te.Code != "resource_unavailable" || te.Message != ac24Message || te.RequestID == "" {
		t.Fatalf("sql_query on %s: error %+v; want code resource_unavailable, message %q, and a request_id", ac24Resource, te, ac24Message)
	}

	logs, err := env.Logs(ctx, harness.ContainerSAG)
	if err != nil {
		t.Fatalf("logs: %v", err)
	}
	ac24AssertClean(t, "podman logs sag", logs, secrets)

	ac24AssertAuditRecord(ctx, t, env, te.RequestID, secrets)
}
