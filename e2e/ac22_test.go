//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kanthorlabs/sag/e2e/harness"
	"github.com/kanthorlabs/sag/internal/testenv"
)

const (
	ac22Sub      = "ac22"
	ac22Resource = "ac22"
	ac22Database = "sag_ac22"
	ac22Limit    = 3 * time.Second
	ac22Hold     = "DO $$ BEGIN LOCK TABLE t_lock22 IN ACCESS EXCLUSIVE MODE; PERFORM pg_sleep(60); END $$"
	ac22Granted  = "SELECT count(*) FROM pg_locks l JOIN pg_class c ON c.oid = l.relation WHERE c.relname = 't_lock22' AND l.mode = 'AccessExclusiveLock' AND l.granted"
)

func init() {
	harness.Register(harness.AC{
		ID:     "AC22",
		Phase:  harness.PhaseParallel,
		Agents: []harness.AgentSpec{{Sub: ac22Sub, Name: "AC22"}},
		Resources: []harness.ResourceSpec{{
			Name:       ac22Resource,
			Containers: []string{harness.ContainerSAG},
			Database:   ac22Database,
			Agents:     []string{ac22Sub},
		}},
		Fixture: "CREATE TABLE t_lock22 (id int); INSERT INTO t_lock22 VALUES (1)",
		Run:     runAC22,
	})
}

func ac22Error(t *testing.T, res *mcp.CallToolResult) conformanceToolError {
	t.Helper()
	var te conformanceToolError
	if !res.IsError {
		return te
	}
	if err := json.Unmarshal([]byte(conformanceText(res)), &te); err != nil {
		t.Fatalf("error text %q: %v", conformanceText(res), err)
	}
	return te
}

func ac22WaitLock(ctx context.Context, t *testing.T, pg *testenv.Postgres) {
	t.Helper()
	conn, err := harness.ConnectDB(ctx, pg, ac22Database)
	if err != nil {
		t.Fatalf("lock probe connect: %v", err)
	}
	defer conn.Close(ctx)
	deadline := time.Now().Add(30 * time.Second)
	for {
		var n int
		if err := conn.QueryRow(ctx, ac22Granted).Scan(&n); err != nil {
			t.Fatalf("lock probe: %v", err)
		}
		if n > 0 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("lock holder holds no ACCESS EXCLUSIVE lock on t_lock22")
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func runAC22(t *testing.T, env harness.Env) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	session, err := env.Client(ctx, harness.ContainerSAG, ac22Sub)
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	defer session.Close()

	deadline := time.Now().Add(70 * time.Second)
	for {
		res := conformanceQuery(ctx, t, session, ac22Resource, "SELECT id FROM t_lock22")
		te := ac22Error(t, res)
		if te.Code == "resource_unavailable" && time.Now().Before(deadline) {
			time.Sleep(2 * time.Second)
			continue
		}
		if res.IsError {
			t.Fatalf("probe query: error %+v", te)
		}
		break
	}

	pg := env.Postgres(env.Major())
	holder, err := harness.ConnectDB(ctx, pg, ac22Database)
	if err != nil {
		t.Fatalf("lock holder connect: %v", err)
	}
	holdCtx, release := context.WithCancel(ctx)
	held := make(chan error, 1)
	go func() {
		held <- holder.PgConn().ExecParams(holdCtx, ac22Hold, nil, nil, nil, nil).Read().Err
	}()
	t.Cleanup(func() {
		release()
		<-held
		closeCtx, closeCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer closeCancel()
		_ = holder.Close(closeCtx)
	})
	ac22WaitLock(ctx, t, pg)

	start := time.Now()
	res := conformanceQuery(ctx, t, session, ac22Resource, "SELECT * FROM t_lock22")
	elapsed := time.Since(start)
	te := ac22Error(t, res)
	if te.Code != "timeout" {
		t.Fatalf("sql_query under ACCESS EXCLUSIVE: result %s; want code timeout", conformanceText(res))
	}
	if elapsed >= ac22Limit {
		t.Fatalf("sql_query under ACCESS EXCLUSIVE took %s; want less than %s", elapsed, ac22Limit)
	}
}
