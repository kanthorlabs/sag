//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/kanthorlabs/sag/e2e/harness"
)

const (
	ac14Sub      = "ac14"
	ac14Resource = "ac14"
	ac14Database = "sag_ac14"
	ac14Marker   = "ac14_probe_5e1"
	ac14Locking  = "SELECT * FROM ac14_t AS " + ac14Marker + " FOR UPDATE"
	ac14Reason   = "SelectStmt.locking_clause"
	ac14WalkMark = "ac14_probe_9b4"
	ac14Sample   = "SELECT * FROM ac14_t AS " + ac14WalkMark + " TABLESAMPLE SYSTEM (50)"
	ac14WalkWhy  = "node:RangeTableSample"
)

func init() {
	harness.Register(harness.AC{
		ID:     "AC14",
		Phase:  harness.PhaseParallel,
		Agents: []harness.AgentSpec{{Sub: ac14Sub, Name: "AC14"}},
		Resources: []harness.ResourceSpec{{
			Name:       ac14Resource,
			Containers: []string{harness.ContainerSAG},
			Database:   ac14Database,
			Agents:     []string{ac14Sub},
		}},
		Fixture: "CREATE TABLE ac14_t (id int)",
		Run:     runAC14,
	})
}

func runAC14(t *testing.T, env harness.Env) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	session, err := env.Client(ctx, harness.ContainerSAG, ac14Sub)
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	defer session.Close()

	start := time.Now()
	deadline := start.Add(70 * time.Second)
	reasons := map[string]string{}
	for _, c := range []struct{ text, reason string }{{ac14Locking, ac14Reason}, {ac14Sample, ac14WalkWhy}} {
		for {
			res := conformanceQuery(ctx, t, session, ac14Resource, c.text)
			if !res.IsError {
				t.Fatalf("sql_query %q: no error result: %s", c.text, conformanceText(res))
			}
			var te conformanceToolError
			if err := json.Unmarshal([]byte(conformanceText(res)), &te); err != nil {
				t.Fatalf("sql_query %q: error text: %v", c.text, err)
			}
			if te.Code == "resource_unavailable" && time.Now().Before(deadline) {
				time.Sleep(2 * time.Second)
				continue
			}
			if te.Code != "query_rejected" || te.Message != rejectionMessage(c.reason) {
				t.Fatalf("sql_query %q: error %+v, want code query_rejected and message %q", c.text, te, rejectionMessage(c.reason))
			}
			reasons[te.RequestID] = c.reason
			break
		}
	}
	assertAuditReasons(ctx, t, env, harness.ContainerSAG, reasons)

	logs, err := env.Postgres(env.Major()).Logs(ctx, start.Add(-time.Second))
	if err != nil {
		t.Fatalf("PostgreSQL logs: %v", err)
	}
	for _, marker := range []string{ac14Marker, ac14WalkMark} {
		if strings.Contains(logs, marker) {
			t.Fatalf("PostgreSQL log holds %q; the rejected statement reached the server:\n%s", marker, logs)
		}
	}
}
