//go:build integration

package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/kanthorlabs/sag/internal/core"
	"github.com/kanthorlabs/sag/internal/secret"
	"github.com/kanthorlabs/sag/internal/sql"
	"github.com/kanthorlabs/sag/internal/testenv"
)

var testLimits = core.Limits{MaxRows: 100, Timeout: 15 * time.Second}

func testResourceConfig(pg *testenv.Postgres, poolSize int) core.ResourceConfig {
	return core.ResourceConfig{
		Name:                            "main",
		Kind:                            Kind,
		TLS:                             core.TLSVerifyFull,
		TLSCA:                           pg.CAPEM,
		PoolSize:                        poolSize,
		ConnectTimeout:                  10 * time.Second,
		LockTimeout:                     2 * time.Second,
		IdleInTransactionSessionTimeout: 20 * time.Second,
	}
}

func openPool(t *testing.T, pg *testenv.Postgres, cfg core.ResourceConfig) sql.Pool {
	t.Helper()
	cc, err := connConfig(cfg, secret.NewCredential("conn-main", []byte(pg.HostDSN())))
	if err != nil {
		t.Fatalf("connConfig: %v", err)
	}
	p, err := newPool(context.Background(), cc, cfg)
	if err != nil {
		t.Fatalf("newPool: %v", err)
	}
	t.Cleanup(p.Close)
	return p
}

func acquire(t *testing.T, ctx context.Context, p sql.Pool) sql.Conn {
	t.Helper()
	c, err := p.Acquire(ctx)
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	return c
}

func fixture(t *testing.T, pg *testenv.Postgres, statements ...string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	conn, err := pg.Connect(ctx)
	if err != nil {
		t.Fatalf("fixture connect: %v", err)
	}
	defer closeConn(ctx, conn)
	for _, s := range statements {
		if err := execText(ctx, conn, s); err != nil {
			t.Fatalf("fixture %q: %v", s, err)
		}
	}
}

func fixtureString(t *testing.T, pg *testenv.Postgres, query string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	conn, err := pg.Connect(ctx)
	if err != nil {
		t.Fatalf("fixture connect: %v", err)
	}
	defer closeConn(ctx, conn)
	var s string
	if err := conn.QueryRow(ctx, query).Scan(&s); err != nil {
		t.Fatalf("fixture %q: %v", query, err)
	}
	return s
}

func beginRequest(t *testing.T, ctx context.Context, c sql.Conn, lim core.Limits, cfg core.ResourceConfig) {
	t.Helper()
	for _, s := range requestBegin(lim, cfg) {
		if err := c.Exec(ctx, s); err != nil {
			t.Fatalf("RequestBegin %q: %v", s, err)
		}
	}
}

func rollback(t *testing.T, ctx context.Context, c sql.Conn) {
	t.Helper()
	if err := c.Exec(ctx, "ROLLBACK"); err != nil {
		t.Fatalf("ROLLBACK: %v", err)
	}
}

func queryValues(ctx context.Context, c sql.Conn, text string, args ...any) ([]any, error) {
	rows, err := c.Query(ctx, text, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var first []any
	for rows.Next() {
		v, err := rows.Values()
		if err != nil {
			return nil, err
		}
		if first == nil {
			first = v
		}
	}
	rows.Close()
	return first, rows.Err()
}

func show(t *testing.T, ctx context.Context, c sql.Conn, name string) string {
	t.Helper()
	v, err := queryValues(ctx, c, "SELECT current_setting($1)", name)
	if err != nil {
		t.Fatalf("current_setting(%s): %v", name, err)
	}
	s, ok := v[0].(string)
	if !ok {
		t.Fatalf("current_setting(%s) = %#v; want string", name, v[0])
	}
	return s
}

func assertEngineError(t *testing.T, err error, state string, class sql.Class) {
	t.Helper()
	var engineErr *sql.EngineError
	if !errors.As(err, &engineErr) || engineErr.SQLState != state || engineErr.Class != class {
		t.Fatalf("error = %v; want EngineError{%s, %s}", err, state, class)
	}
}

func TestSessionGuardReadOnly(t *testing.T) {
	pg := startPostgres(t)
	fixture(t, pg, "CREATE TABLE sg_rows (id int)", "INSERT INTO sg_rows VALUES (1)")
	cfg := testResourceConfig(pg, 2)
	p := openPool(t, pg, cfg)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	c := acquire(t, ctx, p)
	defer c.Close()

	if got := show(t, ctx, c, "default_transaction_read_only"); got != "on" {
		t.Fatalf("default_transaction_read_only = %q; want on", got)
	}
	assertEngineError(t, c.Exec(ctx, "CREATE TABLE sg_probe (id int)"), sqlStateReadOnly, sql.ReadOnlyViolation)
	assertEngineError(t, c.ExecParams(ctx, "INSERT INTO sg_rows VALUES ($1)", 2), sqlStateReadOnly, sql.ReadOnlyViolation)
	_, err := queryValues(ctx, c, "DELETE FROM sg_rows RETURNING id")
	assertEngineError(t, err, sqlStateReadOnly, sql.ReadOnlyViolation)

	if got := fixtureString(t, pg, "SELECT count(*)::text FROM sg_rows"); got != "1" {
		t.Fatalf("sg_rows count = %s; want 1", got)
	}
	if got := fixtureString(t, pg, "SELECT coalesce(to_regclass('sg_probe')::text, 'absent')"); got != "absent" {
		t.Fatalf("sg_probe = %s; want absent", got)
	}
}

func TestRequestBeginSettingsOnServer(t *testing.T) {
	pg := startPostgres(t)
	fixture(t, pg, "ALTER ROLE CURRENT_USER SET datestyle = 'SQL, DMY'", "ALTER ROLE CURRENT_USER SET bytea_output = 'escape'")
	cfg := testResourceConfig(pg, 1)
	p := openPool(t, pg, cfg)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	c := acquire(t, ctx, p)
	defer c.Release()

	beginRequest(t, ctx, c, testLimits, cfg)
	want := map[string]string{
		"transaction_read_only":               "on",
		"standard_conforming_strings":         "on",
		"client_encoding":                     "UTF8",
		"statement_timeout":                   "15s",
		"lock_timeout":                        "2s",
		"idle_in_transaction_session_timeout": "20s",
		"datestyle":                           "ISO, YMD",
		"bytea_output":                        "hex",
	}
	for name, value := range want {
		if got := show(t, ctx, c, name); got != value {
			t.Errorf("PostgreSQL %d: %s = %q; want %q", pg.Major, name, got, value)
		}
	}
	values, err := queryValues(ctx, c, "SELECT '2024-01-02 03:04:05+00'::timestamptz, '2024-01-02'::date, '\\x0102'::bytea")
	if err != nil {
		t.Fatalf("value query: %v", err)
	}
	if ts, ok := values[0].(time.Time); !ok || !ts.Equal(time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)) {
		t.Errorf("PostgreSQL %d: timestamptz = %#v; want 2024-01-02T03:04:05Z", pg.Major, values[0])
	}
	if d, ok := values[1].(time.Time); !ok || d.Month() != time.January || d.Day() != 2 {
		t.Errorf("PostgreSQL %d: date = %#v; want 2024-01-02", pg.Major, values[1])
	}
	if b, ok := values[2].([]byte); !ok || string(b) != "\x01\x02" {
		t.Errorf("PostgreSQL %d: bytea = %#v; want 2 bytes", pg.Major, values[2])
	}
	rollback(t, ctx, c)
	if got := show(t, ctx, c, "statement_timeout"); got != "0" {
		t.Fatalf("statement_timeout after ROLLBACK = %q; want 0", got)
	}
}

func Test25006IsReadOnlyViolation(t *testing.T) {
	pg := startPostgres(t)
	fixture(t, pg,
		"CREATE TABLE ro_rows (id int)",
		"INSERT INTO ro_rows VALUES (1)",
		"CREATE SEQUENCE ro_seq",
	)
	cfg := testResourceConfig(pg, 1)
	p := openPool(t, pg, cfg)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	c := acquire(t, ctx, p)
	defer c.Close()

	for _, text := range []string{
		"INSERT INTO ro_rows VALUES (2)",
		"UPDATE ro_rows SET id = 3",
		"DELETE FROM ro_rows",
		"CREATE TABLE ro_new (id int)",
		"SELECT nextval('ro_seq')",
	} {
		beginRequest(t, ctx, c, testLimits, cfg)
		_, err := queryValues(ctx, c, text)
		assertEngineError(t, err, sqlStateReadOnly, sql.ReadOnlyViolation)
		rollback(t, ctx, c)
	}
	if got := fixtureString(t, pg, "SELECT string_agg(id::text, ',') FROM ro_rows"); got != "1" {
		t.Fatalf("ro_rows = %s; want 1", got)
	}
}

func TestExtendedProtocolOneStatement(t *testing.T) {
	pg := startPostgres(t)
	fixture(t, pg, "CREATE TABLE ext_rows (id int)", "INSERT INTO ext_rows VALUES (1)")
	cfg := testResourceConfig(pg, 1)
	p := openPool(t, pg, cfg)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	c := acquire(t, ctx, p)
	defer c.Close()

	if err := c.Exec(ctx, "SET default_transaction_read_only = off"); err != nil {
		t.Fatal(err)
	}
	const multi = "SELECT 1; DELETE FROM ext_rows"
	assertEngineError(t, c.Exec(ctx, multi), "42601", sql.Other)
	assertEngineError(t, c.ExecParams(ctx, multi), "42601", sql.Other)
	_, err := queryValues(ctx, c, multi)
	assertEngineError(t, err, "42601", sql.Other)
	if got := fixtureString(t, pg, "SELECT count(*)::text FROM ext_rows"); got != "1" {
		t.Fatalf("ext_rows count = %s; want 1", got)
	}
}

func TestAfterConnectRunsSessionGuard(t *testing.T) {
	pg := startPostgres(t)
	cfg := testResourceConfig(pg, 2)
	p := openPool(t, pg, cfg)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	a := acquire(t, ctx, p)
	b := acquire(t, ctx, p)
	for _, c := range []sql.Conn{a, b} {
		if got := show(t, ctx, c, "default_transaction_read_only"); got != "on" {
			t.Fatalf("default_transaction_read_only = %q; want on", got)
		}
	}
	if err := a.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	b.Release()
	c := acquire(t, ctx, p)
	d := acquire(t, ctx, p)
	for _, conn := range []sql.Conn{c, d} {
		if got := show(t, ctx, conn, "default_transaction_read_only"); got != "on" {
			t.Fatalf("default_transaction_read_only after reconnect = %q; want on", got)
		}
		conn.Release()
	}
}

func TestAcquireExhaustedPoolTimesOut(t *testing.T) {
	pg := startPostgres(t)
	p := openPool(t, pg, testResourceConfig(pg, 1))
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	held := acquire(t, ctx, p)
	defer held.Release()

	waitCtx, waitCancel := context.WithTimeout(ctx, 300*time.Millisecond)
	defer waitCancel()
	_, err := p.Acquire(waitCtx)
	var coreErr *core.Error
	if !errors.As(err, &coreErr) || coreErr.Code != core.CodeTimeout {
		t.Fatalf("Acquire error = %v; want core.Error %s", err, core.CodeTimeout)
	}
}

func TestStatementTimeoutIsTimeoutClass(t *testing.T) {
	pg := startPostgres(t)
	cfg := testResourceConfig(pg, 1)
	p := openPool(t, pg, cfg)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	c := acquire(t, ctx, p)
	defer c.Close()

	beginRequest(t, ctx, c, core.Limits{MaxRows: 1, Timeout: 200 * time.Millisecond}, cfg)
	_, err := queryValues(ctx, c, "SELECT pg_sleep(5)")
	assertEngineError(t, err, "57014", sql.Timeout)
}

func TestRowsColumnsAndValues(t *testing.T) {
	pg := startPostgres(t)
	cfg := testResourceConfig(pg, 1)
	p := openPool(t, pg, cfg)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	c := acquire(t, ctx, p)
	defer c.Release()

	beginRequest(t, ctx, c, testLimits, cfg)
	defer rollback(t, ctx, c)
	rows, err := c.Query(ctx, `SELECT 1::int4 AS i, true AS b, 1.5::float8 AS f, 'x'::text AS s,
		'\x0102'::bytea AS raw, '2024-01-02 03:04:05+00'::timestamptz AS ts, 1.25::numeric AS n,
		NULL::text AS empty, '{1,2}'::int4[] AS arr`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	wantColumns := []core.Column{
		{Name: "i", Type: "int4"}, {Name: "b", Type: "bool"}, {Name: "f", Type: "float8"}, {Name: "s", Type: "text"},
		{Name: "raw", Type: "bytea"}, {Name: "ts", Type: "timestamptz"}, {Name: "n", Type: "numeric"},
		{Name: "empty", Type: "text"}, {Name: "arr", Type: "_int4"},
	}
	gotColumns := rows.Columns()
	if len(gotColumns) != len(wantColumns) {
		t.Fatalf("Columns() = %+v; want %+v", gotColumns, wantColumns)
	}
	for i := range wantColumns {
		if gotColumns[i] != wantColumns[i] {
			t.Fatalf("Columns()[%d] = %+v; want %+v", i, gotColumns[i], wantColumns[i])
		}
	}
	if !rows.Next() {
		t.Fatalf("Next() = false; err %v", rows.Err())
	}
	v, err := rows.Values()
	if err != nil {
		t.Fatal(err)
	}
	if v[0] != int64(1) || v[1] != true || v[2] != 1.5 || v[3] != "x" || string(v[4].([]byte)) != "\x01\x02" ||
		!v[5].(time.Time).Equal(time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)) || v[6] != "1.25" || v[7] != nil || v[8] != "{1,2}" {
		t.Fatalf("Values() = %#v", v)
	}
	if rows.Next() {
		t.Fatal("Next() = true; want one row")
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
}

func closeConn(ctx context.Context, conn *pgx.Conn) {
	closeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), closeTimeout)
	defer cancel()
	conn.Close(closeCtx)
}

func execText(ctx context.Context, conn *pgx.Conn, sql string) error {
	return conn.PgConn().ExecParams(ctx, sql, nil, nil, nil, nil).Read().Err
}
