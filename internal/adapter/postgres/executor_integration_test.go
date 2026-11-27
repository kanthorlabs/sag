//go:build integration

package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
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

const productAdvisoryKey = 938001

func productExecutor(t *testing.T, pg *testenv.Postgres) core.Resource {
	t.Helper()
	d, p := openParts(t, pg, testResourceConfig(pg, 1))
	res := sql.NewExecutor(d, p)
	t.Cleanup(func() { res.Close() })
	return res
}

func assertProductRejects(t *testing.T, res core.Resource, text, reason string) {
	t.Helper()
	_, err := runQuery(t, res, text, testLimits)
	var ce *core.Error
	if !errors.As(err, &ce) || ce.Code != core.CodeQueryRejected || ce.Message != rejectionMessage(reason) {
		t.Fatalf("Execute(%q) = %v; want query_rejected %q", text, err, rejectionMessage(reason))
	}
	assertRejectionReason(t, err, reason)
}

func advisoryLocks(t *testing.T, pg *testenv.Postgres, key int) string {
	t.Helper()
	return fixtureString(t, pg, "SELECT count(*)::text FROM pg_catalog.pg_locks WHERE locktype = 'advisory' AND objid = "+strconv.Itoa(key))
}

func TestProductRejectsFieldSelectionCalls(t *testing.T) {
	pg := startPostgres(t)
	fixture(t, pg,
		"CREATE TABLE t38_t (id int, k text)",
		"INSERT INTO t38_t VALUES (1, 'a')",
		"CREATE FUNCTION public.t38_one(int) RETURNS int LANGUAGE plpgsql VOLATILE AS $$BEGIN PERFORM pg_catalog.pg_advisory_lock("+strconv.Itoa(productAdvisoryKey)+"); RETURN $1; END$$",
		"CREATE FUNCTION public.t38_full(t38_t) RETURNS int LANGUAGE plpgsql VOLATILE AS $$BEGIN PERFORM pg_catalog.pg_advisory_lock("+strconv.Itoa(productAdvisoryKey)+"); RETURN 1; END$$",
		"CREATE FUNCTION public.t38_eq(int, int) RETURNS boolean LANGUAGE sql IMMUTABLE AS 'SELECT $1 = $2'",
		"CREATE OPERATOR public.=== (LEFTARG = int, RIGHTARG = int, FUNCTION = public.t38_eq)",
	)
	res := productExecutor(t, pg)
	out, err := runQuery(t, res, "SELECT t.id, t.k FROM t38_t t", testLimits)
	if err != nil || len(out.Rows) != 1 {
		t.Fatalf("qualified column read = %+v, %v; want one row", out, err)
	}
	for text, reason := range map[string]string{
		"SELECT (SELECT 938001).pg_advisory_lock":       "function_prefix:pg_advisory_lock",
		"SELECT ('ssl_key_file'::text).current_setting": "function_denied:current_setting",
		"SELECT ('PG_VERSION'::text).pg_read_file":      "function_prefix:pg_read_file",
		"SELECT ('SELECT 1'::text).ts_stat":             "function_denied:ts_stat",
		"SELECT (7::int).t38_one":                       "function_catalog:t38_one",
		"SELECT t.t38_full FROM t38_t t":                "function_catalog:t38_full",
		"SELECT (0.5::float8).setseed":                  "function_volatile:setseed",
		"SELECT setseed(0.5)":                           "function_volatile:setseed",
		"SELECT t38_one(1)":                             "function_catalog:t38_one",
		"SELECT 1 === 1":                                "operator_catalog:===",
	} {
		assertProductRejects(t, res, text, reason)
	}
	if n := advisoryLocks(t, pg, productAdvisoryKey); n != "0" {
		t.Fatalf("pg_locks holds %s advisory locks on key %d; want 0", n, productAdvisoryKey)
	}
}

func TestProductJoinUsingResolvesEquals(t *testing.T) {
	pg := startPostgres(t)
	fixture(t, pg,
		"CREATE EXTENSION citext",
		"CREATE TABLE t38_c1 (k citext)",
		"CREATE TABLE t38_c2 (k citext)",
		"INSERT INTO t38_c1 VALUES ('A')",
		"INSERT INTO t38_c2 VALUES ('a')",
	)
	res := productExecutor(t, pg)
	for _, text := range []string{
		"SELECT * FROM t38_c1 JOIN t38_c2 USING (k)",
		"SELECT * FROM t38_c1 NATURAL JOIN t38_c2",
		"SELECT * FROM t38_c1 a JOIN t38_c2 b ON a.k = b.k",
	} {
		assertProductRejects(t, res, text, "operator_catalog:=")
	}
	out, err := runQuery(t, res, "SELECT * FROM t38_c1 CROSS JOIN t38_c2", testLimits)
	if err != nil || len(out.Rows) != 1 {
		t.Fatalf("CROSS JOIN = %+v, %v; want one row", out, err)
	}
}
