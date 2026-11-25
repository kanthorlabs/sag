//go:build e2e

package e2e

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"regexp"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kanthorlabs/sag/e2e/harness"
)

const (
	ac18Sub         = "ac18"
	ac18Resource    = "ac18"
	ac18Database    = "sag_e2e"
	ac18Statement   = "SELECT * FROM v"
	ac18Unavailable = "resource_unavailable"
	ac18Rows        = 5
)

var ac18BlockedInsert = regexp.MustCompile(
	`ERROR:\s+cannot execute INSERT in a read-only transaction\r?\n[^\n]*CONTEXT:\s+` +
		regexp.QuoteMeta(`SQL statement "INSERT INTO public.ac18_t VALUES (18001, 'ac18_write')"`),
)

func init() {
	harness.Register(harness.AC{
		ID:     "AC18",
		Phase:  harness.PhaseParallel,
		Agents: []harness.AgentSpec{{Sub: ac18Sub, Name: "AC18"}},
		Resources: []harness.ResourceSpec{{
			Name:       ac18Resource,
			Containers: []string{harness.ContainerSAG},
			Database:   ac18Database,
			Agents:     []string{ac18Sub},
		}},
		Fixture: "CREATE TABLE public.ac18_t (id int PRIMARY KEY, v text); " +
			"INSERT INTO public.ac18_t SELECT g, 'v' || g FROM generate_series(1, 5) g; " +
			"CREATE FUNCTION public.f_insert() RETURNS SETOF public.ac18_t LANGUAGE plpgsql VOLATILE AS " +
			"$$BEGIN INSERT INTO public.ac18_t VALUES (18001, 'ac18_write'); RETURN QUERY SELECT * FROM public.ac18_t; END$$; " +
			"CREATE VIEW public.v AS SELECT * FROM public.f_insert()",
		Run: runAC18,
	})
}

type ac18ToolError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type ac18State struct {
	Rows     int64
	Checksum string
}

func ac18Expected() ac18State {
	sum := md5.Sum([]byte("(1,v1)|(2,v2)|(3,v3)|(4,v4)|(5,v5)"))
	return ac18State{Rows: ac18Rows, Checksum: hex.EncodeToString(sum[:])}
}

func ac18TableState(ctx context.Context, t *testing.T, env harness.Env) ac18State {
	t.Helper()
	conn, err := harness.ConnectDB(ctx, env.Postgres(env.Major()), ac18Database)
	if err != nil {
		t.Fatalf("connect %s: %v", ac18Database, err)
	}
	defer conn.Close(context.WithoutCancel(ctx))
	var s ac18State
	err = conn.QueryRow(ctx,
		"SELECT count(*), md5(coalesce(string_agg(t::text, '|' ORDER BY t::text), '')) FROM public.ac18_t t",
	).Scan(&s.Rows, &s.Checksum)
	if err != nil {
		t.Fatalf("state of ac18_t: %v", err)
	}
	return s
}

func ac18Query(ctx context.Context, t *testing.T, session *mcp.ClientSession) (ac18ToolError, string) {
	t.Helper()
	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "sql_query",
		Arguments: map[string]any{"resource": ac18Resource, "sql": ac18Statement},
	})
	if err != nil {
		t.Fatalf("sql_query %q: %v", ac18Statement, err)
	}
	if len(res.Content) != 1 {
		t.Fatalf("sql_query %q returned %d content items; want 1", ac18Statement, len(res.Content))
	}
	tc, ok := res.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("sql_query %q content is %T", ac18Statement, res.Content[0])
	}
	if !res.IsError {
		t.Fatalf("sql_query %q returned a result; want an error: %s", ac18Statement, tc.Text)
	}
	var out ac18ToolError
	if err := json.Unmarshal([]byte(tc.Text), &out); err != nil {
		t.Fatalf("sql_query %q error %q: %v", ac18Statement, tc.Text, err)
	}
	return out, tc.Text
}

func runAC18(t *testing.T, env harness.Env) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	want := ac18Expected()
	if before := ac18TableState(ctx, t, env); before != want {
		t.Fatalf("ac18_t before the request = %+v; want %+v", before, want)
	}

	session, err := env.Client(ctx, harness.ContainerSAG, ac18Sub)
	if err != nil {
		t.Fatalf("MCP client: %v", err)
	}
	defer session.Close()

	start := time.Now()
	deadline := start.Add(70 * time.Second)
	got, text := ac18Query(ctx, t, session)
	for got.Code == ac18Unavailable {
		if time.Now().After(deadline) {
			t.Fatalf("resource %s is not served after 70 s", ac18Resource)
		}
		time.Sleep(2 * time.Second)
		got, text = ac18Query(ctx, t, session)
	}

	wantErr := ac18ToolError{Code: "upstream_error", Message: "upstream error, SQLSTATE 25006"}
	if got != wantErr {
		t.Errorf("sql_query %q = %s; want %+v", ac18Statement, text, wantErr)
	}

	if after := ac18TableState(ctx, t, env); after != want {
		t.Errorf("ac18_t after the request = %+v; want %+v", after, want)
	}

	logs, err := env.Postgres(env.Major()).Logs(ctx, start.Add(-time.Second))
	if err != nil {
		t.Fatalf("PostgreSQL logs: %v", err)
	}
	if !ac18BlockedInsert.MatchString(logs) {
		t.Errorf("PostgreSQL log has no read-only rejection of the INSERT in f_insert; want a match of %s in:\n%s", ac18BlockedInsert, logs)
	}
}
