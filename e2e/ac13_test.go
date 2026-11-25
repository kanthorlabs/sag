//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kanthorlabs/sag/e2e/harness"
)

const (
	ac13Sub          = "ac13"
	ac13Resource     = "ac13"
	ac13Database     = "sag_ac13"
	ac13FirstNumber  = 913001
	ac13Rejected     = "query_rejected"
	ac13Unavailable  = "resource_unavailable"
	ac13ParseMessage = "statement does not parse"
)

type ac13Case struct {
	text   string
	reason string
}

func ac13Cases() []ac13Case {
	return []ac13Case{
		{"INSERT INTO ac13_t VALUES (913001)", "statement:InsertStmt"},
		{"UPDATE ac13_t SET id = 913002", "statement:UpdateStmt"},
		{"DELETE FROM ac13_t WHERE id = 913003", "statement:DeleteStmt"},
		{"MERGE INTO ac13_t t USING ac13_t s ON t.id = s.id WHEN MATCHED THEN UPDATE SET id = 913004", "statement:MergeStmt"},
		{"TRUNCATE ac13_t_913005", "statement:TruncateStmt"},
		{"DROP TABLE ac13_t_913006", "statement:DropStmt"},
		{"CREATE TABLE ac13_t_913007 (id int)", "statement:CreateStmt"},
		{"CREATE TEMPORARY TABLE ac13_t_913008 (id int)", "statement:CreateStmt"},
		{"ALTER TABLE ac13_t ADD COLUMN c_913009 int", "statement:AlterTableStmt"},
		{"GRANT SELECT ON ac13_t_913010 TO PUBLIC", "statement:GrantStmt"},
		{"COPY (SELECT 913011) TO STDOUT", "statement:CopyStmt"},
		{"SET work_mem = 913012", "statement:VariableSetStmt"},
		{"BEGIN /* 913013 */", "statement:TransactionStmt"},
		{"COMMIT /* 913014 */", "statement:TransactionStmt"},
		{"CALL ac13_p(913015)", "statement:CallStmt"},
		{"DO $$BEGIN PERFORM 913016; END$$", "statement:DoStmt"},
		{"SELECT 1; DELETE FROM ac13_t WHERE id = 913017", "statement_count:2"},
		{"WITH d AS (DELETE FROM ac13_t WHERE id = 913018 RETURNING *) SELECT * FROM d", "statement:DeleteStmt"},
		{"SELECT * INTO ac13_t_913019 FROM ac13_t", "SelectStmt.into_clause"},
		{"SELECT * FROM ac13_t WHERE id = 913020 FOR UPDATE", "SelectStmt.locking_clause"},
		{"EXPLAIN ANALYZE SELECT 913021", "explain_option:analyze"},
		{"EXPLAIN (ANALYZE true) SELECT 913022", "explain_option:analyze"},
		{"/* 913023 */", "statement_count:0"},
		{"SELEC 913024", ""},
	}
}

func init() {
	harness.Register(harness.AC{
		ID:     "AC13",
		Phase:  harness.PhaseParallel,
		Agents: []harness.AgentSpec{{Sub: ac13Sub, Name: "AC13"}},
		Resources: []harness.ResourceSpec{{
			Name:       ac13Resource,
			Containers: []string{harness.ContainerSAG},
			Database:   ac13Database,
			Agents:     []string{ac13Sub},
		}},
		Run: runAC13,
	})
}

type ac13ToolError struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id"`
}

func ac13Query(ctx context.Context, t *testing.T, session *mcp.ClientSession, text string) ac13ToolError {
	t.Helper()
	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "sql_query",
		Arguments: map[string]any{"resource": ac13Resource, "sql": text},
	})
	if err != nil {
		t.Fatalf("sql_query %q: %v", text, err)
	}
	if !res.IsError || len(res.Content) != 1 {
		t.Fatalf("sql_query %q returned a result; want an error: %+v", text, res.Content)
	}
	tc, ok := res.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("sql_query %q error content is %T", text, res.Content[0])
	}
	var out ac13ToolError
	if err := json.Unmarshal([]byte(tc.Text), &out); err != nil {
		t.Fatalf("sql_query %q error %q: %v", text, tc.Text, err)
	}
	return out
}

func ac13WaitServed(ctx context.Context, t *testing.T, session *mcp.ClientSession, probe string) {
	t.Helper()
	deadline := time.Now().Add(70 * time.Second)
	for ac13Query(ctx, t, session, probe).Code == ac13Unavailable {
		if time.Now().After(deadline) {
			t.Fatalf("resource %s is not served after 70 s", ac13Resource)
		}
		time.Sleep(2 * time.Second)
	}
}

func ac13WantMessage(c ac13Case) string {
	if c.reason == "" {
		return ac13ParseMessage
	}
	return rejectionMessage(c.reason)
}

func runAC13(t *testing.T, env harness.Env) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	cases := ac13Cases()
	if len(cases) != 24 {
		t.Fatalf("AC13 list has %d statements; want 24", len(cases))
	}
	for i, c := range cases {
		number := fmt.Sprint(ac13FirstNumber + i)
		if !strings.Contains(c.text, number) {
			t.Fatalf("AC13 statement %d %q does not carry its number %s", i+1, c.text, number)
		}
	}

	session, err := env.Client(ctx, harness.ContainerSAG, ac13Sub)
	if err != nil {
		t.Fatalf("MCP client: %v", err)
	}
	defer session.Close()

	start := time.Now()
	ac13WaitServed(ctx, t, session, cases[0].text)
	reasons := map[string]string{}
	for _, c := range cases {
		got := ac13Query(ctx, t, session, c.text)
		if got.Code != ac13Rejected || got.Message != ac13WantMessage(c) {
			t.Errorf("sql_query %q = %+v; want code %q and message %q", c.text, got, ac13Rejected, ac13WantMessage(c))
		}
		if c.reason != "" {
			reasons[got.RequestID] = c.reason
		}
	}
	assertAuditReasons(ctx, t, env, harness.ContainerSAG, reasons)

	logs, err := env.Postgres(env.Major()).Logs(ctx, start.Add(-time.Second))
	if err != nil {
		t.Fatalf("PostgreSQL logs: %v", err)
	}
	for i := range cases {
		if number := fmt.Sprint(ac13FirstNumber + i); strings.Contains(logs, number) {
			t.Errorf("PostgreSQL log holds statement number %s; SAG sent a rejected statement to the server", number)
		}
	}
}
