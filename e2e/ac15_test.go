//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kanthorlabs/sag/e2e/harness"
)

const (
	ac15Sub         = "ac15"
	ac15Resource    = "ac15"
	ac15Database    = "sag_ac15"
	ac15Rejected    = "query_rejected"
	ac15Unavailable = "resource_unavailable"
	ac15MaxSleep    = 5 * time.Second
)

type ac15Case struct {
	text   string
	reason string
	marker string
	effect func(ctx context.Context, t *testing.T, db *pgx.Conn, logs string)
}

func ac15Cases() []ac15Case {
	return []ac15Case{
		{"SELECT pg_advisory_lock(15001) AS ac15_r01", "function_prefix:pg_advisory_lock", "ac15_r01", ac15NoAdvisoryLock(15001)},
		{"SELECT brin_summarize_new_values('ac15_brin_idx'::regclass) AS ac15_r02", "function_denied:brin_summarize_new_values", "ac15_r02", nil},
		{"SELECT gin_clean_pending_list('ac15_gin_idx'::regclass) AS ac15_r03", "function_denied:gin_clean_pending_list", "ac15_r03", nil},
		{"SELECT * FROM ts_stat('SELECT to_tsvector(''ac15_r04'')')", "function_denied:ts_stat", "ac15_r04", nil},
		{"SELECT pg_sleep(10) AS ac15_r05", "function_prefix:pg_sleep", "ac15_r05", nil},
		{"SELECT pg_read_file('postgresql.conf') AS ac15_r06", "function_prefix:pg_read_file", "ac15_r06", nil},
		{"SELECT pg_terminate_backend(1) AS ac15_r07", "function_prefix:pg_terminate_backend", "ac15_r07", nil},
		{"SELECT pg_reload_conf() AS ac15_r08", "function_prefix:pg_reload_conf", "ac15_r08", ac15NoReload},
		{"COPY (SELECT 1 AS ac15_r09) TO PROGRAM 'touch /tmp/ac15_r09'", "statement:CopyStmt", "ac15_r09", ac15NoFile("/tmp/ac15_r09")},
		{"COPY (SELECT 1 AS ac15_r10) TO '/tmp/ac15_r10'", "statement:CopyStmt", "ac15_r10", ac15NoFile("/tmp/ac15_r10")},
		{"PREPARE TRANSACTION 'ac15_r11'", "statement:TransactionStmt", "ac15_r11", ac15NoPreparedXact},
		{"CHECKPOINT", "statement:CheckPointStmt", "CHECKPOINT", nil},
		{"PREPARE ac15_r13 AS SELECT 1", "statement:PrepareStmt", "ac15_r13", nil},
		{"SELECT * FROM pg_authid AS ac15_r14", "relation:pg_authid", "ac15_r14", nil},
		{"SELECT * FROM pg_shadow AS ac15_r15", "relation:pg_shadow", "ac15_r15", nil},
		{"SELECT * FROM pg_user_mapping AS ac15_r16", "relation:pg_user_mapping", "ac15_r16", nil},
		{"SELECT * FROM pg_user_mappings AS ac15_r17", "relation:pg_user_mappings", "ac15_r17", nil},
		{"SELECT * FROM pg_subscription AS ac15_r18", "relation:pg_subscription", "ac15_r18", nil},
		{"SELECT * FROM pg_settings AS ac15_r19", "relation:pg_settings", "ac15_r19", nil},
		{"SELECT * FROM pg_file_settings AS ac15_r20", "relation:pg_file_settings", "ac15_r20", nil},
		{"SELECT current_setting('primary_conninfo') AS ac15_r21", "function_denied:current_setting", "ac15_r21", nil},
		{"SELECT (SELECT 15022).pg_advisory_lock AS ac15_r22", "function_prefix:pg_advisory_lock", "ac15_r22", ac15NoAdvisoryLock(15022)},
		{"SELECT ('ssl_key_file'::text).current_setting AS ac15_r23", "function_denied:current_setting", "ac15_r23", nil},
		{"SELECT ('PG_VERSION'::text).pg_read_file AS ac15_r24", "function_prefix:pg_read_file", "ac15_r24", nil},
		{"SELECT ('SELECT to_tsvector(''ac15_r25'')'::text).ts_stat", "function_denied:ts_stat", "ac15_r25", nil},
		{"SELECT (7::int).ac15_fn AS ac15_r26", "function_catalog:ac15_fn", "ac15_r26", ac15NoAdvisoryLock(15026)},
		{"SELECT r.ac15_rowfn AS ac15_r27 FROM ac15_row r", "function_catalog:ac15_rowfn", "ac15_r27", ac15NoAdvisoryLock(15026)},
		{"SELECT setseed(0.5) AS ac15_r28", "function_volatile:setseed", "ac15_r28", nil},
		{"SELECT ac15_fn(1) AS ac15_r29", "function_catalog:ac15_fn", "ac15_r29", ac15NoAdvisoryLock(15026)},
		{"SELECT 1 === 1 AS ac15_r30", "operator_catalog:===", "ac15_r30", nil},
		{"SELECT * FROM pg_statistic AS ac15_r31", "relation:pg_statistic", "ac15_r31", nil},
		{"SELECT * FROM pg_stats AS ac15_r32", "relation:pg_stats", "ac15_r32", nil},
		{"SELECT * FROM pg_statistic_ext_data AS ac15_r33", "relation:pg_statistic_ext_data", "ac15_r33", nil},
		{"SELECT * FROM pg_stats_ext AS ac15_r34", "relation:pg_stats_ext", "ac15_r34", nil},
		{"SELECT * FROM pg_stats_ext_exprs AS ac15_r35", "relation:pg_stats_ext_exprs", "ac15_r35", nil},
		{"SELECT * FROM information_schema.user_mapping_options AS ac15_r36", "relation:information_schema.user_mapping_options", "ac15_r36", nil},
		{"SELECT * FROM information_schema._pg_user_mappings AS ac15_r37", "relation:information_schema._pg_user_mappings", "ac15_r37", nil},
	}
}

func init() {
	harness.Register(harness.AC{
		ID:     "AC15",
		Phase:  harness.PhaseParallel,
		Agents: []harness.AgentSpec{{Sub: ac15Sub, Name: "AC15"}},
		Resources: []harness.ResourceSpec{{
			Name:       ac15Resource,
			Containers: []string{harness.ContainerSAG},
			Database:   ac15Database,
			Agents:     []string{ac15Sub},
		}},
		Fixture: "CREATE TABLE ac15_brin (id int); " +
			"CREATE INDEX ac15_brin_idx ON ac15_brin USING brin (id); " +
			"INSERT INTO ac15_brin SELECT g FROM generate_series(1, 1000) g; " +
			"CREATE TABLE ac15_gin (v int[]); " +
			"CREATE INDEX ac15_gin_idx ON ac15_gin USING gin (v) WITH (fastupdate = on); " +
			"INSERT INTO ac15_gin SELECT ARRAY[g] FROM generate_series(1, 100) g; " +
			"CREATE FUNCTION ac15_fn(int) RETURNS int LANGUAGE plpgsql VOLATILE AS $$BEGIN PERFORM pg_advisory_lock(15026); RETURN $1; END$$; " +
			"CREATE TABLE ac15_row (id int); " +
			"INSERT INTO ac15_row VALUES (1); " +
			"CREATE FUNCTION ac15_rowfn(ac15_row) RETURNS int LANGUAGE plpgsql VOLATILE AS $$BEGIN PERFORM pg_advisory_lock(15026); RETURN 1; END$$; " +
			"CREATE FUNCTION ac15_eq(int, int) RETURNS boolean LANGUAGE sql IMMUTABLE AS 'SELECT $1 = $2'; " +
			"CREATE OPERATOR === (LEFTARG = int, RIGHTARG = int, FUNCTION = ac15_eq)",
		Run: runAC15,
	})
}

type ac15ToolError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func ac15Query(ctx context.Context, t *testing.T, session *mcp.ClientSession, text string) ac15ToolError {
	t.Helper()
	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "sql_query",
		Arguments: map[string]any{"resource": ac15Resource, "sql": text},
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
	var out ac15ToolError
	if err := json.Unmarshal([]byte(tc.Text), &out); err != nil {
		t.Fatalf("sql_query %q error %q: %v", text, tc.Text, err)
	}
	return out
}

func ac15WaitServed(ctx context.Context, t *testing.T, session *mcp.ClientSession, probe string) {
	t.Helper()
	deadline := time.Now().Add(70 * time.Second)
	for ac15Query(ctx, t, session, probe).Code == ac15Unavailable {
		if time.Now().After(deadline) {
			t.Fatalf("resource %s is not served after 70 s", ac15Resource)
		}
		time.Sleep(2 * time.Second)
	}
}

func ac15Count(ctx context.Context, t *testing.T, db *pgx.Conn, query string, args ...any) int64 {
	t.Helper()
	var n int64
	if err := db.QueryRow(ctx, query, args...).Scan(&n); err != nil {
		t.Fatalf("effect check %q: %v", query, err)
	}
	return n
}

func ac15NoAdvisoryLock(key int64) func(ctx context.Context, t *testing.T, db *pgx.Conn, logs string) {
	return func(ctx context.Context, t *testing.T, db *pgx.Conn, logs string) {
		if n := ac15Count(ctx, t, db, "SELECT count(*) FROM pg_catalog.pg_locks WHERE locktype = 'advisory' AND objid::int8 = $1", key); n != 0 {
			t.Fatalf("pg_locks holds %d advisory locks on key %d; want 0", n, key)
		}
	}
}

func ac15NoReload(ctx context.Context, t *testing.T, db *pgx.Conn, logs string) {
	if strings.Contains(logs, "received SIGHUP") {
		t.Fatalf("PostgreSQL log shows a configuration reload:\n%s", logs)
	}
}

func ac15NoFile(path string) func(ctx context.Context, t *testing.T, db *pgx.Conn, logs string) {
	return func(ctx context.Context, t *testing.T, db *pgx.Conn, logs string) {
		if n := ac15Count(ctx, t, db, "SELECT count(*) FROM pg_catalog.pg_stat_file($1, true) s WHERE s.size IS NOT NULL", path); n != 0 {
			t.Fatalf("file %s exists on the database host", path)
		}
	}
}

func ac15NoPreparedXact(ctx context.Context, t *testing.T, db *pgx.Conn, logs string) {
	if n := ac15Count(ctx, t, db, "SELECT count(*) FROM pg_catalog.pg_prepared_xacts WHERE gid = 'ac15_r11'"); n != 0 {
		t.Fatalf("pg_prepared_xacts holds %d transactions ac15_r11; want 0", n)
	}
}

func runAC15(t *testing.T, env harness.Env) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()

	session, err := env.Client(ctx, harness.ContainerSAG, ac15Sub)
	if err != nil {
		t.Fatalf("MCP client: %v", err)
	}
	defer session.Close()

	start := time.Now()
	cases := ac15Cases()
	ac15WaitServed(ctx, t, session, cases[0].text)

	for _, c := range cases {
		began := time.Now()
		got := ac15Query(ctx, t, session, c.text)
		elapsed := time.Since(began)
		want := ac15ToolError{Code: ac15Rejected, Message: "statement rejected: " + c.reason}
		if got != want {
			t.Errorf("sql_query %q = %+v; want %+v", c.text, got, want)
		}
		if elapsed > ac15MaxSleep {
			t.Errorf("sql_query %q took %s; want less than %s", c.text, elapsed, ac15MaxSleep)
		}
	}

	pg := env.Postgres(env.Major())
	logs, err := pg.Logs(ctx, start.Add(-time.Second))
	if err != nil {
		t.Fatalf("PostgreSQL logs: %v", err)
	}
	db, err := harness.ConnectDB(ctx, pg, ac15Database)
	if err != nil {
		t.Fatalf("connect %s: %v", ac15Database, err)
	}
	defer db.Close(context.WithoutCancel(ctx))

	for _, c := range cases {
		if strings.Contains(logs, c.marker) {
			t.Errorf("PostgreSQL log holds %q; SAG sent %q to the server", c.marker, c.text)
		}
		if c.effect != nil {
			c.effect(ctx, t, db, logs)
		}
	}
}
