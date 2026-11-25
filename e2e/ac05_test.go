//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kanthorlabs/sag/e2e/harness"
)

const (
	ac05Sub      = "ac05"
	ac05Resource = "ac05"
	ac05Database = "sag_ac05"
	ac05Rows     = 5
)

func init() {
	harness.Register(harness.AC{
		ID:     "AC05",
		Phase:  harness.PhaseParallel,
		Agents: []harness.AgentSpec{{Sub: ac05Sub, Name: "AC05"}},
		Resources: []harness.ResourceSpec{{
			Name:       ac05Resource,
			Containers: []string{harness.ContainerConformance},
			Database:   ac05Database,
			Agents:     []string{ac05Sub},
		}},
		Fixture: "CREATE TABLE ac05_t (id int PRIMARY KEY, v text); INSERT INTO ac05_t SELECT g, 'v' || g FROM generate_series(1, 5) g",
		Run:     runAC05,
	})
}

func runAC05(t *testing.T, env harness.Env) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	before := conformanceTableState(ctx, t, env, ac05Database, "ac05_t")
	if before.Rows != ac05Rows {
		t.Fatalf("ac05_t has %d rows before the test, want %d", before.Rows, ac05Rows)
	}

	session, err := env.Client(ctx, harness.ContainerConformance, ac05Sub)
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	defer session.Close()

	conformanceExpectCount(ctx, t, session, ac05Resource, "SELECT count(*) FROM ac05_t", ac05Rows)

	for _, stmt := range []string{
		"INSERT INTO ac05_t VALUES (99, 'x')",
		"UPDATE ac05_t SET v = 'changed'",
		"DELETE FROM ac05_t",
		"CREATE TABLE ac05_new (id int)",
	} {
		conformanceExpectError(ctx, t, session, ac05Resource, stmt, "upstream_error", "upstream error, SQLSTATE 25006")
	}

	after := conformanceTableState(ctx, t, env, ac05Database, "ac05_t")
	if after != before {
		t.Fatalf("ac05_t changed: before %+v, after %+v", before, after)
	}
	if conformanceTableExists(ctx, t, env, ac05Database, "ac05_new") {
		t.Fatal("table ac05_new exists after CREATE TABLE through sag-conformance")
	}
}

type conformanceToolError struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id"`
}

type conformanceQueryResult struct {
	Rows     [][]any `json:"rows"`
	RowCount int     `json:"row_count"`
}

type conformanceState struct {
	Rows     int64
	Checksum string
}

func conformanceQuery(ctx context.Context, t *testing.T, session *mcp.ClientSession, resource, stmt string) *mcp.CallToolResult {
	t.Helper()
	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "sql_query",
		Arguments: map[string]any{"resource": resource, "sql": stmt},
	})
	if err != nil {
		t.Fatalf("sql_query %q: %v", stmt, err)
	}
	return res
}

func conformanceExpectCount(ctx context.Context, t *testing.T, session *mcp.ClientSession, resource, stmt string, want int) {
	t.Helper()
	res := conformanceQuery(ctx, t, session, resource, stmt)
	if res.IsError {
		t.Fatalf("sql_query %q: error result %s", stmt, conformanceText(res))
	}
	var out conformanceQueryResult
	if err := json.Unmarshal([]byte(conformanceText(res)), &out); err != nil {
		t.Fatalf("sql_query %q: result text: %v", stmt, err)
	}
	if out.RowCount != 1 || len(out.Rows) != 1 || len(out.Rows[0]) != 1 {
		t.Fatalf("sql_query %q: result %+v, want one row with one column", stmt, out)
	}
	if n, ok := out.Rows[0][0].(float64); !ok || n != float64(want) {
		t.Fatalf("sql_query %q: value %v, want %d", stmt, out.Rows[0][0], want)
	}
}

func conformanceExpectError(ctx context.Context, t *testing.T, session *mcp.ClientSession, resource, stmt, wantCode, wantMessage string) {
	t.Helper()
	res := conformanceQuery(ctx, t, session, resource, stmt)
	if !res.IsError {
		t.Fatalf("sql_query %q: no error result: %s", stmt, conformanceText(res))
	}
	var te conformanceToolError
	if err := json.Unmarshal([]byte(conformanceText(res)), &te); err != nil {
		t.Fatalf("sql_query %q: error text: %v", stmt, err)
	}
	if te.Code != wantCode || te.Message != wantMessage {
		t.Fatalf("sql_query %q: error %+v, want code %q and message %q", stmt, te, wantCode, wantMessage)
	}
}

func conformanceText(res *mcp.CallToolResult) string {
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			return tc.Text
		}
	}
	return ""
}

func conformanceTableState(ctx context.Context, t *testing.T, env harness.Env, database, table string) conformanceState {
	t.Helper()
	conn, err := harness.ConnectDB(ctx, env.Postgres(env.Major()), database)
	if err != nil {
		t.Fatalf("connect %s: %v", database, err)
	}
	defer conn.Close(context.WithoutCancel(ctx))
	var s conformanceState
	err = conn.QueryRow(ctx,
		"SELECT count(*), md5(coalesce(string_agg(t::text, '|' ORDER BY t::text), '')) FROM "+table+" t",
	).Scan(&s.Rows, &s.Checksum)
	if err != nil {
		t.Fatalf("state of %s: %v", table, err)
	}
	return s
}

func conformanceTableExists(ctx context.Context, t *testing.T, env harness.Env, database, table string) bool {
	t.Helper()
	conn, err := harness.ConnectDB(ctx, env.Postgres(env.Major()), database)
	if err != nil {
		t.Fatalf("connect %s: %v", database, err)
	}
	defer conn.Close(context.WithoutCancel(ctx))
	var exists bool
	if err := conn.QueryRow(ctx, "SELECT to_regclass($1) IS NOT NULL", "public."+table).Scan(&exists); err != nil {
		t.Fatalf("to_regclass %s: %v", table, err)
	}
	return exists
}
