//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kanthorlabs/sag/e2e/harness"
)

const (
	ac24Sub      = "ac24"
	ac24Resource = "badpass"
	ac24Rotated  = "rotated"
	ac24Role     = "sag_ac24_rotated"
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
		}, {
			Name:       ac24Rotated,
			Containers: []string{harness.ContainerSAG},
			Role:       ac24Role,
			Agents:     []string{ac24Sub},
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

func ac24Secrets(ctx context.Context, t *testing.T, env harness.Env, resource string) []string {
	t.Helper()
	dsn, err := harness.ReadContainerFile(ctx, env.ContainerName(harness.ContainerSAG), "/run/secrets/"+harness.ConnectionSecret(resource))
	if err != nil {
		t.Fatalf("read connection secret: %v", err)
	}
	u, err := url.Parse(strings.TrimSpace(string(dsn)))
	if err != nil {
		t.Fatal("connection secret does not parse as a URL")
	}
	password, ok := u.User.Password()
	if !ok || len(password) < 16 {
		t.Fatal("connection secret has no password of 16 or more characters")
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

func ac24AssertAuditRecord(ctx context.Context, t *testing.T, env harness.Env, resource, requestID string, secrets []string) {
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
			if rec.Resource != resource || rec.Decision != "deny" || rec.Reason != "resource_unavailable" {
				t.Fatalf("audit record %+v; want resource %s, decision deny, reason resource_unavailable", rec, resource)
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

	secrets := ac24Secrets(ctx, t, env, ac24Resource)
	ac24WaitPreflightFailed(ctx, t, env)

	session, err := env.Client(ctx, harness.ContainerSAG, ac24Sub)
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	defer session.Close()

	te := ac24Unavailable(ctx, t, session, ac24Resource, secrets)

	logs, err := env.Logs(ctx, harness.ContainerSAG)
	if err != nil {
		t.Fatalf("logs: %v", err)
	}
	ac24AssertClean(t, "podman logs sag", logs, secrets)

	ac24AssertAuditRecord(ctx, t, env, ac24Resource, te.RequestID, secrets)

	t.Run("password changed after preflight", func(t *testing.T) {
		runAC24Rotated(ctx, t, env, session)
	})
}

func ac24Unavailable(ctx context.Context, t *testing.T, session *mcp.ClientSession, resource string, secrets []string) ac24ToolError {
	t.Helper()
	res := conformanceQuery(ctx, t, session, resource, "SELECT 1")
	text := conformanceText(res)
	ac24AssertClean(t, "response", text, secrets)
	var te ac24ToolError
	if !res.IsError {
		t.Fatalf("sql_query on %s: no error result: %s", resource, text)
	}
	if err := json.Unmarshal([]byte(text), &te); err != nil {
		t.Fatalf("error text %q: %v", text, err)
	}
	if te.Code != "resource_unavailable" || te.Message != ac24Message || te.RequestID == "" {
		t.Fatalf("sql_query on %s: error %+v; want code resource_unavailable, message %q, and a request_id", resource, te, ac24Message)
	}
	return te
}

func ac24WaitServed(ctx context.Context, t *testing.T, session *mcp.ClientSession) {
	t.Helper()
	deadline := time.Now().Add(70 * time.Second)
	for {
		res := conformanceQuery(ctx, t, session, ac24Rotated, "SELECT 1")
		if !res.IsError {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("resource %s is not served: %s", ac24Rotated, conformanceText(res))
		}
		time.Sleep(2 * time.Second)
	}
}

func ac24AdminExec(ctx context.Context, t *testing.T, admin *pgx.Conn, stmt string) {
	t.Helper()
	if _, err := admin.PgConn().Exec(ctx, stmt).ReadAll(); err != nil {
		t.Fatalf("admin statement on role %s: %v", ac24Role, err)
	}
}

func ac24SetPassword(role, password string) string {
	return "ALTER ROLE " + pgx.Identifier{role}.Sanitize() + " PASSWORD '" + password + "'"
}

func runAC24Rotated(ctx context.Context, t *testing.T, env harness.Env, session *mcp.ClientSession) {
	secrets := ac24Secrets(ctx, t, env, ac24Rotated)
	ac24WaitServed(ctx, t, session)

	admin, err := harness.ConnectDB(ctx, env.Postgres(env.Major()), "postgres")
	if err != nil {
		t.Fatalf("admin connect: %v", err)
	}
	t.Cleanup(func() {
		cleanCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		ac24AdminExec(cleanCtx, t, admin, ac24SetPassword(ac24Role, secrets[0]))
		_ = admin.Close(cleanCtx)
	})

	time.Sleep(2 * time.Second)
	ac24AdminExec(ctx, t, admin, ac24SetPassword(ac24Role, strings.Repeat("0", 32)))
	if _, err := admin.Exec(ctx, "SELECT pg_terminate_backend(pid, 5000) FROM pg_stat_activity WHERE usename = $1", ac24Role); err != nil {
		t.Fatalf("terminate backends of role %s: %v", ac24Role, err)
	}

	te := ac24Unavailable(ctx, t, session, ac24Rotated, secrets)

	logs, err := env.Logs(ctx, harness.ContainerSAG)
	if err != nil {
		t.Fatalf("logs: %v", err)
	}
	ac24AssertClean(t, "podman logs sag", logs, secrets)
	if !slices.ContainsFunc(strings.Split(logs, "\n"), func(line string) bool {
		return strings.Contains(line, `"msg":"request resource unavailable"`) && strings.Contains(line, te.RequestID) && strings.Contains(line, "28P01")
	}) {
		t.Fatalf("daemon log has no driver authentication cause (28P01) for request_id %s", te.RequestID)
	}

	ac24AssertAuditRecord(ctx, t, env, ac24Rotated, te.RequestID, secrets)
}
