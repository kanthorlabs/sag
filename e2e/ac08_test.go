//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kanthorlabs/sag/e2e/harness"
)

const (
	ac08Sub      = "ac08"
	ac08Resource = "ac08"
	ac08Database = "sag_ac08"
	ac08Rows     = 5
)

var (
	ac08Statement = regexp.MustCompile(`(?m)LOG:\s+(?:statement|execute [^:]*):\s?(.*)$`)
	ac08Begin     = regexp.MustCompile(`(?i)^\s*(BEGIN|START\s+TRANSACTION)\b`)
	ac08ReadOnly  = regexp.MustCompile(`(?i)\bREAD\s+ONLY\b`)
	ac08Rollback  = regexp.MustCompile(`(?i)^\s*ROLLBACK\s*;?\s*$`)
	ac08Commit    = regexp.MustCompile(`(?i)\bCOMMIT\b|^\s*END\b`)
)

func init() {
	harness.Register(harness.AC{
		ID:     "AC08",
		Phase:  harness.PhaseSequential,
		Agents: []harness.AgentSpec{{Sub: ac08Sub, Name: "AC08"}},
		Resources: []harness.ResourceSpec{{
			Name:       ac08Resource,
			Containers: []string{harness.ContainerConformance},
			Database:   ac08Database,
			Agents:     []string{ac08Sub},
		}},
		Fixture: "CREATE TABLE ac08_t (id int); INSERT INTO ac08_t SELECT generate_series(1, 5)",
		Run:     runAC08,
	})
}

type ac08ToolError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func runAC08(t *testing.T, env harness.Env) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	session, err := env.Client(ctx, harness.ContainerConformance, ac08Sub)
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	defer session.Close()

	ac08Call(ctx, t, session, "sql_query", map[string]any{"sql": "SELECT id FROM ac08_t"}, "")
	ac08Call(ctx, t, session, "sql_query", map[string]any{"sql": "INSERT INTO ac08_t VALUES (99)"}, "upstream_error")
	ac08Call(ctx, t, session, "sql_query", map[string]any{"sql": "SELECT * FROM ac08_missing"}, "upstream_error")
	ac08Call(ctx, t, session, "sql_list_tables", map[string]any{}, "")
	const requests = 4

	pg := env.Postgres(env.Major())
	conn, err := harness.ConnectDB(ctx, pg, ac08Database)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	var n int
	err = conn.QueryRow(ctx, "SELECT count(*) FROM ac08_t").Scan(&n)
	_ = conn.Close(ctx)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != ac08Rows {
		t.Fatalf("ac08_t has %d rows, want %d", n, ac08Rows)
	}

	var begins, rollbacks int
	deadline := time.Now().Add(30 * time.Second)
	for {
		logs, err := pg.Logs(ctx, time.Time{})
		if err != nil {
			t.Fatalf("postgres logs: %v", err)
		}
		begins, rollbacks = ac08Count(t, logs)
		if begins == rollbacks || time.Now().After(deadline) {
			break
		}
		time.Sleep(time.Second)
	}
	if begins != rollbacks {
		t.Fatalf("BEGIN count %d, ROLLBACK count %d", begins, rollbacks)
	}
	if begins < requests {
		t.Fatalf("BEGIN count %d, want at least %d", begins, requests)
	}
}

func ac08Call(ctx context.Context, t *testing.T, session *mcp.ClientSession, tool string, args map[string]any, wantCode string) {
	t.Helper()
	args["resource"] = ac08Resource
	res, err := session.CallTool(ctx, &mcp.CallToolParams{Name: tool, Arguments: args})
	if err != nil {
		t.Fatalf("%s %v: %v", tool, args, err)
	}
	if wantCode == "" {
		if res.IsError {
			t.Fatalf("%s %v: error result %s", tool, args, ac08Text(res))
		}
		return
	}
	if !res.IsError {
		t.Fatalf("%s %v: no error result", tool, args)
	}
	var te ac08ToolError
	if err := json.Unmarshal([]byte(ac08Text(res)), &te); err != nil {
		t.Fatalf("%s %v: error text: %v", tool, args, err)
	}
	if te.Code != wantCode || !strings.Contains(te.Message, "SQLSTATE ") {
		t.Fatalf("%s %v: error = %+v, want code %s with SQLSTATE", tool, args, te, wantCode)
	}
}

func ac08Text(res *mcp.CallToolResult) string {
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			return tc.Text
		}
	}
	return ""
}

func ac08Count(t *testing.T, logs string) (begins, rollbacks int) {
	t.Helper()
	for _, m := range ac08Statement.FindAllStringSubmatch(logs, -1) {
		stmt := m[1]
		if ac08Commit.MatchString(stmt) {
			t.Fatalf("server log holds COMMIT: %q", stmt)
		}
		switch {
		case ac08Begin.MatchString(stmt):
			if !ac08ReadOnly.MatchString(stmt) {
				t.Fatalf("transaction is not READ ONLY: %q", stmt)
			}
			begins++
		case ac08Rollback.MatchString(stmt):
			rollbacks++
		}
	}
	return begins, rollbacks
}
