//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kanthorlabs/sag/e2e/harness"
)

const (
	ac16Sub           = "ac16"
	ac16Resource      = "ac16"
	ac16Database      = "sag_ac16"
	ac16FixedSCSMajor = 19
	ac16Unavailable   = "resource_unavailable"
	ac16MaxDuration   = time.Second
	ac16HeldLocks     = "SELECT count(*) FROM pg_catalog.pg_locks l JOIN pg_catalog.pg_stat_activity a ON a.pid = l.pid WHERE a.pid <> pg_catalog.pg_backend_pid() AND (l.locktype = 'advisory' OR (a.datname = $1 AND a.state = 'idle'))"
)

type ac16Case struct {
	text string
	want []string
}

func ac16Cases() []ac16Case {
	return []ac16Case{
		{`SELECT 'a\', ' , pg_sleep(2) -- '`, []string{`a\`, ` , pg_sleep(2) -- `}},
		{`SELECT 'a\', ' , pg_advisory_lock(916) -- '`, []string{`a\`, ` , pg_advisory_lock(916) -- `}},
		{`SELECT E'\\', ' , pg_sleep(2) -- '`, []string{`\`, ` , pg_sleep(2) -- `}},
		{`SELECT $$a\', pg_sleep(2) -- $$, U&'\005C', 'ソ\', /* x /* y */ ', pg_sleep(2) -- */ 'b'`, []string{`a\', pg_sleep(2) -- `, `\`, `ソ\`, `b`}},
	}
}

func init() {
	harness.Register(harness.AC{
		ID:     "AC16",
		Phase:  harness.PhaseSequential,
		Agents: []harness.AgentSpec{{Sub: ac16Sub, Name: "AC16"}},
		Resources: []harness.ResourceSpec{{
			Name:       ac16Resource,
			Containers: []string{harness.ContainerSAG},
			Database:   ac16Database,
			Agents:     []string{ac16Sub},
		}},
		Run: runAC16,
	})
}

type ac16Output struct {
	Code    string  `json:"code"`
	Message string  `json:"message"`
	Rows    [][]any `json:"rows"`
}

func ac16Connect(ctx context.Context, t *testing.T, env harness.Env) *pgx.Conn {
	t.Helper()
	conn, err := harness.ConnectDB(ctx, env.Postgres(env.Major()), ac16Database)
	if err != nil {
		t.Fatalf("connect %s: %v", ac16Database, err)
	}
	return conn
}

func ac16Admin(ctx context.Context, t *testing.T, env harness.Env, statement string) error {
	t.Helper()
	conn := ac16Connect(ctx, t, env)
	defer conn.Close(context.WithoutCancel(ctx))
	return conn.PgConn().ExecParams(ctx, statement, nil, nil, nil, nil).Read().Err
}

func ac16Setting(ctx context.Context, t *testing.T, env harness.Env) string {
	t.Helper()
	conn := ac16Connect(ctx, t, env)
	defer conn.Close(context.WithoutCancel(ctx))
	var value string
	if err := conn.QueryRow(ctx, "SELECT pg_catalog.current_setting($1)", "standard_conforming_strings").Scan(&value); err != nil {
		t.Fatalf("current_setting: %v", err)
	}
	return value
}

func ac16Locks(ctx context.Context, t *testing.T, env harness.Env) int {
	t.Helper()
	conn := ac16Connect(ctx, t, env)
	defer conn.Close(context.WithoutCancel(ctx))
	var n int
	if err := conn.QueryRow(ctx, ac16HeldLocks, ac16Database).Scan(&n); err != nil {
		t.Fatalf("pg_locks: %v", err)
	}
	return n
}

func ac16SetLexer(ctx context.Context, t *testing.T, env harness.Env) {
	t.Helper()
	set := []string{
		"ALTER ROLE postgres SET standard_conforming_strings = off",
		"ALTER DATABASE " + ac16Database + " SET standard_conforming_strings = off",
	}
	reset := []string{
		"ALTER ROLE postgres RESET standard_conforming_strings",
		"ALTER DATABASE " + ac16Database + " RESET standard_conforming_strings",
	}
	t.Cleanup(func() {
		cctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		for _, statement := range reset {
			if err := ac16Admin(cctx, t, env, statement); err != nil {
				t.Errorf("%s: %v", statement, err)
			}
		}
	})
	for _, statement := range set {
		err := ac16Admin(ctx, t, env, statement)
		switch {
		case env.Major() < ac16FixedSCSMajor && err != nil:
			t.Fatalf("%s on PostgreSQL %d: %v", statement, env.Major(), err)
		case env.Major() >= ac16FixedSCSMajor && err == nil:
			t.Fatalf("%s on PostgreSQL %d succeeded; want an error", statement, env.Major())
		}
	}
	want := "off"
	if env.Major() >= ac16FixedSCSMajor {
		want = "on"
	}
	if got := ac16Setting(ctx, t, env); got != want {
		t.Fatalf("new session standard_conforming_strings = %q; want %q", got, want)
	}
}

func ac16Query(ctx context.Context, t *testing.T, session *mcp.ClientSession, text string) (ac16Output, bool, time.Duration) {
	t.Helper()
	start := time.Now()
	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "sql_query",
		Arguments: map[string]any{"resource": ac16Resource, "sql": text},
	})
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("sql_query %q: %v", text, err)
	}
	if len(res.Content) != 1 {
		t.Fatalf("sql_query %q content = %+v; want one item", text, res.Content)
	}
	tc, ok := res.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("sql_query %q content is %T", text, res.Content[0])
	}
	var out ac16Output
	if err := json.Unmarshal([]byte(tc.Text), &out); err != nil {
		t.Fatalf("sql_query %q content %q: %v", text, tc.Text, err)
	}
	return out, res.IsError, elapsed
}

func ac16Served(ctx context.Context, t *testing.T, session *mcp.ClientSession, text string) (ac16Output, time.Duration) {
	t.Helper()
	deadline := time.Now().Add(70 * time.Second)
	for {
		out, isError, elapsed := ac16Query(ctx, t, session, text)
		if !isError {
			return out, elapsed
		}
		if out.Code != ac16Unavailable {
			t.Fatalf("sql_query %q = %s %q; want a result", text, out.Code, out.Message)
		}
		if time.Now().After(deadline) {
			t.Fatalf("resource %s is not served after 70 s", ac16Resource)
		}
		time.Sleep(2 * time.Second)
	}
}

func runAC16(t *testing.T, env harness.Env) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	ac16SetLexer(ctx, t, env)
	if n := ac16Locks(ctx, t, env); n != 0 {
		t.Fatalf("pg_locks holds %d advisory or idle-session locks before the requests; want 0", n)
	}

	session, err := env.Client(ctx, harness.ContainerSAG, ac16Sub)
	if err != nil {
		t.Fatalf("MCP client: %v", err)
	}
	defer session.Close()

	for _, c := range ac16Cases() {
		out, elapsed := ac16Served(ctx, t, session, c.text)
		if len(out.Rows) != 1 {
			t.Fatalf("sql_query %q rows = %v; want one row", c.text, out.Rows)
		}
		got := make([]string, len(out.Rows[0]))
		for i, v := range out.Rows[0] {
			s, ok := v.(string)
			if !ok {
				t.Fatalf("sql_query %q column %d = %#v; want a string", c.text, i, v)
			}
			got[i] = s
		}
		if !slices.Equal(got, c.want) {
			t.Errorf("sql_query %q row = %q; want %q, the values that the validator read", c.text, got, c.want)
		}
		if elapsed >= ac16MaxDuration {
			t.Errorf("sql_query %q took %s; want less than %s", c.text, elapsed, ac16MaxDuration)
		}
	}

	if n := ac16Locks(ctx, t, env); n != 0 {
		t.Fatalf("pg_locks holds %d advisory or idle-session locks after the requests; want 0", n)
	}
}
