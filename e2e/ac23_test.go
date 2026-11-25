//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kanthorlabs/sag/e2e/harness"
)

const (
	ac23Sub         = "ac23"
	ac23ResourceA   = "rate-a"
	ac23ResourceB   = "rate-b"
	ac23Database    = "sag_ac23"
	ac23Select      = "SELECT id FROM ac23_t"
	ac23Lock        = "LOCK TABLE ac23_t IN ACCESS EXCLUSIVE MODE"
	ac23Waiters     = "SELECT count(*) FROM pg_catalog.pg_stat_activity WHERE datname = $1 AND wait_event_type = 'Lock'"
	ac23ServeWindow = 70 * time.Second
	ac23WaitWindow  = 1500 * time.Millisecond
	ac23Rounds      = 3
)

func init() {
	harness.Register(harness.AC{
		ID:     "AC23",
		Phase:  harness.PhaseParallel,
		Agents: []harness.AgentSpec{{Sub: ac23Sub, Name: "AC23"}},
		Resources: []harness.ResourceSpec{
			{
				Name:       ac23ResourceA,
				Containers: []string{harness.ContainerRate},
				Database:   ac23Database,
				Agents:     []string{ac23Sub},
			},
			{
				Name:       ac23ResourceB,
				Containers: []string{harness.ContainerRate},
				Database:   ac23Database,
				Agents:     []string{ac23Sub},
			},
		},
		Fixture: "CREATE TABLE IF NOT EXISTS ac23_t (id int); INSERT INTO ac23_t SELECT 1 WHERE NOT EXISTS (SELECT 1 FROM ac23_t)",
		Run:     runAC23,
	})
}

type ac23Output struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type ac23Held struct {
	code string
	done time.Time
}

func ac23Call(ctx context.Context, session *mcp.ClientSession, resource string) (string, error) {
	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "sql_query",
		Arguments: map[string]any{"resource": resource, "sql": ac23Select},
	})
	if err != nil {
		return "", err
	}
	if !res.IsError {
		return "", nil
	}
	if len(res.Content) != 1 {
		return "", nil
	}
	tc, ok := res.Content[0].(*mcp.TextContent)
	if !ok {
		return "", nil
	}
	var out ac23Output
	if err := json.Unmarshal([]byte(tc.Text), &out); err != nil {
		return "", err
	}
	return out.Code, nil
}

func ac23Sessions(ctx context.Context, t *testing.T, env harness.Env, n int) []*mcp.ClientSession {
	t.Helper()
	sessions := make([]*mcp.ClientSession, n)
	for i := range sessions {
		s, err := env.Client(ctx, harness.ContainerRate, ac23Sub)
		if err != nil {
			t.Fatalf("MCP client: %v", err)
		}
		t.Cleanup(func() { _ = s.Close() })
		sessions[i] = s
	}
	return sessions
}

func runAC23(t *testing.T, env harness.Env) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	sessions := ac23Sessions(ctx, t, env, 6)

	for _, resource := range []string{ac23ResourceA, ac23ResourceB} {
		deadline := time.Now().Add(ac23ServeWindow)
		for {
			code, err := ac23Call(ctx, sessions[0], resource)
			if err != nil {
				t.Fatalf("sql_query on %s: %v", resource, err)
			}
			if code == "" {
				break
			}
			if code != "resource_unavailable" || time.Now().After(deadline) {
				t.Fatalf("sql_query on %s = %q; want a result", resource, code)
			}
			time.Sleep(2 * time.Second)
		}
	}

	var last string
	for round := 1; round <= ac23Rounds; round++ {
		ok, detail := ac23Round(ctx, t, env, sessions)
		if ok {
			return
		}
		last = detail
		t.Logf("round %d: %s", round, detail)
	}
	t.Fatalf("no round proved rate_limited for a fifth request: %s", last)
}

func ac23Round(ctx context.Context, t *testing.T, env harness.Env, sessions []*mcp.ClientSession) (bool, string) {
	t.Helper()
	conn, err := harness.ConnectDB(ctx, env.Postgres(env.Major()), ac23Database)
	if err != nil {
		t.Fatalf("connect %s: %v", ac23Database, err)
	}
	defer conn.Close(context.Background())
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	rolledBack := false
	rollback := func() {
		if rolledBack {
			return
		}
		rolledBack = true
		if err := tx.Rollback(context.Background()); err != nil {
			t.Errorf("rollback: %v", err)
		}
	}
	defer rollback()
	if _, err := tx.Exec(ctx, ac23Lock); err != nil {
		t.Fatalf("%s: %v", ac23Lock, err)
	}

	held := make([]ac23Held, 4)
	var wg sync.WaitGroup
	for i, resource := range []string{ac23ResourceA, ac23ResourceA, ac23ResourceB, ac23ResourceB} {
		wg.Go(func() {
			code, err := ac23Call(ctx, sessions[i], resource)
			if err != nil {
				code = "call error: " + err.Error()
			}
			held[i] = ac23Held{code: code, done: time.Now()}
		})
	}

	deadline := time.Now().Add(ac23WaitWindow)
	waiting := 0
	for waiting < 4 && time.Now().Before(deadline) {
		if err := tx.QueryRow(ctx, ac23Waiters, ac23Database).Scan(&waiting); err != nil {
			t.Fatalf("count lock waiters: %v", err)
		}
		if waiting < 4 {
			time.Sleep(20 * time.Millisecond)
		}
	}
	if waiting < 4 {
		rollback()
		wg.Wait()
		return false, "the four requests did not wait on the lock together"
	}

	fifth := make([]string, 2)
	var fwg sync.WaitGroup
	for i, resource := range []string{ac23ResourceA, ac23ResourceB} {
		fwg.Go(func() {
			code, err := ac23Call(ctx, sessions[4+i], resource)
			if err != nil {
				code = "call error: " + err.Error()
			}
			fifth[i] = code
		})
	}
	fwg.Wait()
	fifthDone := time.Now()
	rollback()
	wg.Wait()

	for i, h := range held {
		if h.code == "rate_limited" {
			t.Fatalf("held request %d = rate_limited; want it to pass the semaphore", i)
		}
		if !h.done.After(fifthDone) {
			return false, "a held request ended before the fifth requests returned"
		}
	}
	for i, code := range fifth {
		if code != "rate_limited" {
			t.Fatalf("fifth request on %s = %q; want rate_limited", []string{ac23ResourceA, ac23ResourceB}[i], code)
		}
	}
	return true, ""
}
