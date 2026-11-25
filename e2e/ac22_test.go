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
	ac22Sub      = "ac22"
	ac22Resource = "ac22"
	ac22Database = "sag_ac22"
	ac22Limit    = 3 * time.Second
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

	holder, err := harness.ConnectDB(ctx, env.Postgres(env.Major()), ac22Database)
	if err != nil {
		t.Fatalf("lock holder connect: %v", err)
	}
	t.Cleanup(func() {
		closeCtx, closeCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer closeCancel()
		_ = holder.PgConn().ExecParams(closeCtx, "ROLLBACK", nil, nil, nil, nil).Read().Err
		_ = holder.Close(closeCtx)
	})
	for _, s := range []string{"BEGIN", "LOCK TABLE t_lock22 IN ACCESS EXCLUSIVE MODE"} {
		if err := holder.PgConn().ExecParams(ctx, s, nil, nil, nil, nil).Read().Err; err != nil {
			t.Fatalf("lock holder %q: %v", s, err)
		}
	}

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
