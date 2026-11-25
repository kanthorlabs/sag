//go:build integration

package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kanthorlabs/sag/e2e/conformance/passthrough"
	"github.com/kanthorlabs/sag/internal/core"
	"github.com/kanthorlabs/sag/internal/secret"
	"github.com/kanthorlabs/sag/internal/sql"
	"github.com/kanthorlabs/sag/internal/testenv"
)

func openParts(t *testing.T, pg *testenv.Postgres, cfg core.ResourceConfig) (*dialect, sql.Pool) {
	t.Helper()
	d, p, err := openSQL(context.Background(), cfg, secret.NewCredential("conn-main", []byte(pg.HostDSN())))
	if err != nil {
		t.Fatalf("openSQL: %v", err)
	}
	return d, p
}

func passthroughExecutor(t *testing.T, pg *testenv.Postgres, cfg core.ResourceConfig) core.Resource {
	t.Helper()
	d, p := openParts(t, pg, cfg)
	res := sql.NewExecutor(passthrough.Wrap(d), p)
	t.Cleanup(func() { res.Close() })
	return res
}

func runQuery(t *testing.T, res core.Resource, text string, lim core.Limits) (core.Result, error) {
	t.Helper()
	raw, err := json.Marshal(map[string]string{"resource": "main", "sql": text})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	return res.Execute(ctx, sql.CapabilityName, sql.OperationQuery, raw, lim)
}

func backendPID(t *testing.T, res core.Resource) any {
	t.Helper()
	out, err := runQuery(t, res, "SELECT pg_catalog.pg_backend_pid()", testLimits)
	if err != nil || len(out.Rows) != 1 {
		t.Fatalf("pg_backend_pid: %+v, %v", out, err)
	}
	return out.Rows[0][0]
}

func TestTruncatedReadWithoutCursorLeavesPool(t *testing.T) {
	pg := startPostgres(t)
	res := passthroughExecutor(t, pg, testResourceConfig(pg, 1))
	before := backendPID(t, res)
	start := time.Now()
	out, err := runQuery(t, res, "SELECT pg_catalog.generate_series(1, 40000000)", core.Limits{MaxRows: 5, Timeout: 15 * time.Second})
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("truncated read: %v", err)
	}
	if len(out.Rows) != 5 || !out.Truncated || out.TruncatedReason != core.TruncatedMaxRows {
		t.Fatalf("result = %d rows, truncated %v %q; want 5 rows truncated by max_rows", len(out.Rows), out.Truncated, out.TruncatedReason)
	}
	if elapsed > time.Second {
		t.Fatalf("truncated read took %s; want less than 1 s", elapsed)
	}
	if after := backendPID(t, res); after == before {
		t.Fatalf("backend PID %v after a truncated read; want a new connection", after)
	}
}

func TestPreflightPassesWithFullPool(t *testing.T) {
	pg := startPostgres(t)
	cfg := testResourceConfig(pg, 1)
	d, p := openParts(t, pg, cfg)
	res := sql.NewExecutor(d, p)
	defer res.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	held, err := p.Acquire(ctx)
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	defer held.Release()
	report, err := res.Preflight(ctx)
	if err != nil || !report.Passed {
		t.Fatalf("Preflight with a full pool = %+v, %v; want passed", report, err)
	}
}

func TestStatementTimeoutRollsBack(t *testing.T) {
	pg := startPostgres(t)
	res := passthroughExecutor(t, pg, testResourceConfig(pg, 1))
	start := time.Now()
	_, err := runQuery(t, res, "SELECT pg_catalog.pg_sleep(30)", core.Limits{MaxRows: 10, Timeout: time.Second})
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("timeout took %s", elapsed)
	}
	var ce *core.Error
	var ee *sql.EngineError
	if !errors.As(err, &ce) || ce.Code != core.CodeTimeout || !errors.As(err, &ee) || ee.SQLState != "57014" {
		t.Fatalf("err = %v; want timeout with SQLSTATE 57014", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	logs, err := pg.Logs(ctx, start)
	if err != nil {
		t.Fatalf("Logs: %v", err)
	}
	if b, r := strings.Count(logs, "BEGIN TRANSACTION READ ONLY"), strings.Count(logs, "ROLLBACK"); b != 1 || r != 1 {
		t.Fatalf("server log has %d BEGIN and %d ROLLBACK; want 1 each:\n%s", b, r, logs)
	}
}
