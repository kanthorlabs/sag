//go:build e2e

package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/kanthorlabs/sag/e2e/harness"
)

const (
	ac06Sub      = "ac06"
	ac06Resource = "ac06"
	ac06Database = "sag_ac06"
	ac06Rows     = 5
)

func init() {
	harness.Register(harness.AC{
		ID:     "AC06",
		Phase:  harness.PhaseParallel,
		Agents: []harness.AgentSpec{{Sub: ac06Sub, Name: "AC06"}},
		Resources: []harness.ResourceSpec{{
			Name:       ac06Resource,
			Containers: []string{harness.ContainerConformance},
			Database:   ac06Database,
			Agents:     []string{ac06Sub},
		}},
		Fixture: "CREATE TABLE ac06_t (id int PRIMARY KEY); INSERT INTO ac06_t SELECT generate_series(1, 5)",
		Run:     runAC06,
	})
}

func runAC06(t *testing.T, env harness.Env) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	before := conformanceTableState(ctx, t, env, ac06Database, "ac06_t")
	if before.Rows != ac06Rows {
		t.Fatalf("ac06_t has %d rows before the test, want %d", before.Rows, ac06Rows)
	}

	session, err := env.Client(ctx, harness.ContainerConformance, ac06Sub)
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	defer session.Close()

	conformanceExpectCount(ctx, t, session, ac06Resource, "SELECT count(*) FROM ac06_t", ac06Rows)
	conformanceExpectError(ctx, t, session, ac06Resource, "SELECT 1; DELETE FROM ac06_t", "upstream_error", "upstream error, SQLSTATE 42601")

	after := conformanceTableState(ctx, t, env, ac06Database, "ac06_t")
	if after != before {
		t.Fatalf("ac06_t changed: before %+v, after %+v", before, after)
	}
}
