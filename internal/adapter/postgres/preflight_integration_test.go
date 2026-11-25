//go:build integration

package postgres

import (
	"bytes"
	"context"
	"errors"
	"net/url"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kanthorlabs/sag/internal/approval"
	"github.com/kanthorlabs/sag/internal/core"
	"github.com/kanthorlabs/sag/internal/secret"
	"github.com/kanthorlabs/sag/internal/sql"
	"github.com/kanthorlabs/sag/internal/testenv"
)

func startPostgres(t *testing.T) *testenv.Postgres {
	t.Helper()
	major, err := testenv.MajorFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	pg, err := testenv.StartPostgres(ctx, major)
	if err != nil {
		t.Fatalf("StartPostgres: %v", err)
	}
	t.Cleanup(func() { pg.Close(context.Background()) })
	return pg
}

func openResource(t *testing.T, cfg core.ResourceConfig, dsn string) core.Resource {
	t.Helper()
	res, err := Adapter{}.Open(context.Background(), cfg, secret.NewCredential("sag-main", []byte(dsn)))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { res.Close() })
	return res
}

func TestPreflightMinimal(t *testing.T) {
	pg := startPostgres(t)
	start := time.Now()
	cfg := core.ResourceConfig{Name: "main", Kind: Kind, TLS: core.TLSVerifyFull, TLSCA: pg.CAPEM, PoolSize: 2, ConnectTimeout: 10 * time.Second}
	res := openResource(t, cfg, pg.HostDSN())

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	report, err := res.Preflight(ctx)
	if err != nil {
		t.Fatalf("Preflight: %v", err)
	}
	if !report.Passed {
		t.Fatalf("Preflight report = %+v; want passed", report)
	}
	engine := false
	for _, c := range report.Checks {
		if c.Name == checkEngine && c.Passed {
			engine = true
		}
	}
	if !engine {
		t.Fatalf("Preflight checks = %+v; want engine passed", report.Checks)
	}

	logs, err := pg.Logs(ctx, start)
	if err != nil {
		t.Fatalf("Logs: %v", err)
	}
	begin := strings.Index(logs, "BEGIN TRANSACTION READ ONLY")
	version := strings.Index(logs, "pg_catalog.version()")
	rollback := strings.Index(logs, "ROLLBACK")
	if begin < 0 || version < begin || rollback < version {
		t.Fatalf("server log lacks BEGIN READ ONLY, pg_catalog.version(), ROLLBACK in order:\n%s", logs)
	}
	if strings.Contains(logs, "COMMIT") {
		t.Fatalf("server log has COMMIT:\n%s", logs)
	}
}

func TestPreflightRefusesUnknownCA(t *testing.T) {
	pg := startPostgres(t)
	other, err := testenv.NewCA()
	if err != nil {
		t.Fatal(err)
	}
	cfg := core.ResourceConfig{Name: "main", Kind: Kind, TLS: core.TLSVerifyFull, TLSCA: other.CertPEM, PoolSize: 1, ConnectTimeout: 10 * time.Second}
	res := openResource(t, cfg, pg.HostDSN())
	_, err = res.Preflight(context.Background())
	if err == nil {
		t.Fatal("Preflight passed with a CA that did not sign the server certificate")
	}
	assertPreflightCause(t, err, preflightTLSFailed)
}

func assertPreflightCause(t *testing.T, err error, want string) {
	t.Helper()
	var coreErr *core.Error
	if !errors.As(err, &coreErr) || coreErr.Code != core.CodeResourceUnavailable || coreErr.Message != want {
		t.Fatalf("Preflight error = %v; want core.Error %s with message %q", err, core.CodeResourceUnavailable, want)
	}
}

func TestExtendedProtocolRejectsMultiStatement(t *testing.T) {
	pg := startPostgres(t)
	cfg := core.ResourceConfig{Name: "main", Kind: Kind, TLS: core.TLSVerifyFull, TLSCA: pg.CAPEM, PoolSize: 1, ConnectTimeout: 10 * time.Second}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_, p, err := openSQL(ctx, cfg, secret.NewCredential("sag-main", []byte(pg.HostDSN())))
	if err != nil {
		t.Fatalf("openSQL: %v", err)
	}
	defer p.Close()
	conn, err := p.Acquire(ctx)
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	defer conn.Close()
	err = conn.Exec(ctx, "CREATE TEMP TABLE s2_probe (id int); INSERT INTO s2_probe VALUES (1); SELECT 1")
	var engineErr *sql.EngineError
	if !errors.As(err, &engineErr) || engineErr.SQLState != "42601" {
		t.Fatalf("multi-statement error = %v; want SQLSTATE 42601", err)
	}
}

func TestPreflightWrongPasswordOmitsPassword(t *testing.T) {
	pg := startPostgres(t)
	u, err := url.Parse(pg.HostDSN())
	if err != nil {
		t.Fatal(err)
	}
	const wrong = "wrong-Pw_7731"
	u.User = url.UserPassword(u.User.Username(), wrong)
	cfg := core.ResourceConfig{Name: "main", Kind: Kind, TLS: core.TLSVerifyFull, TLSCA: pg.CAPEM, PoolSize: 1, ConnectTimeout: 10 * time.Second}
	res := openResource(t, cfg, u.String())
	_, err = res.Preflight(context.Background())
	if err == nil {
		t.Fatal("Preflight passed with a wrong password")
	}
	if strings.Contains(err.Error(), wrong) {
		t.Fatalf("error text leaks the password: %q", err.Error())
	}
	assertPreflightCause(t, err, preflightConnectFailed)
}

const preflightTestTimeout = time.Minute

func preflightResourceConfig(pg *testenv.Postgres) core.ResourceConfig {
	return core.ResourceConfig{
		Name:                            "main",
		Kind:                            Kind,
		TLS:                             core.TLSVerifyFull,
		TLSCA:                           pg.CAPEM,
		PoolSize:                        2,
		ConnectTimeout:                  10 * time.Second,
		LockTimeout:                     2 * time.Second,
		IdleInTransactionSessionTimeout: 20 * time.Second,
	}
}

func preflightCred(pg *testenv.Postgres) secret.Credential {
	return secret.NewCredential("conn-main", []byte(pg.HostDSN()))
}

func runSQLPreflight(t *testing.T, pg *testenv.Postgres, cfg core.ResourceConfig) (core.PreflightReport, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), preflightTestTimeout)
	defer cancel()
	d, p, err := Adapter{}.OpenSQL(ctx, cfg, preflightCred(pg))
	if err != nil {
		t.Fatalf("OpenSQL: %v", err)
	}
	res := sql.NewExecutor(d, p)
	defer res.Close()
	return res.Preflight(ctx)
}

func mustSQLPreflight(t *testing.T, pg *testenv.Postgres, cfg core.ResourceConfig) core.PreflightReport {
	t.Helper()
	report, err := runSQLPreflight(t, pg, cfg)
	if err != nil {
		t.Fatalf("Preflight: %v", err)
	}
	return report
}

func preflightFixture(t *testing.T, pg *testenv.Postgres, statements ...string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), preflightTestTimeout)
	defer cancel()
	conn, err := pg.Connect(ctx)
	if err != nil {
		t.Fatalf("fixture connect: %v", err)
	}
	defer conn.Close(ctx)
	for _, s := range statements {
		if _, err := conn.PgConn().ExecParams(ctx, s, nil, nil, nil, nil).Close(); err != nil {
			t.Fatalf("fixture %q: %v", s, err)
		}
	}
}

func TestRefusesPG13(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	pg, err := testenv.StartPostgres(ctx, 13)
	if err != nil {
		t.Fatalf("StartPostgres(13): %v", err)
	}
	t.Cleanup(func() { pg.Close(context.Background()) })
	report := mustSQLPreflight(t, pg, preflightResourceConfig(pg))
	if report.Passed || reportCheck(t, report, checkVersion).Passed {
		t.Fatalf("report = %+v; want version refused on PostgreSQL 13", report)
	}
	if !reportCheck(t, report, checkEngine).Passed {
		t.Fatalf("report = %+v; want engine passed on PostgreSQL 13", report)
	}
}

func TestRefusesUnknownCA(t *testing.T) {
	pg := startPostgres(t)
	other, err := testenv.NewCA()
	if err != nil {
		t.Fatal(err)
	}
	cfg := preflightResourceConfig(pg)
	cfg.TLSCA = other.CertPEM
	report, err := runSQLPreflight(t, pg, cfg)
	var coreErr *core.Error
	if report.Passed || !errors.As(err, &coreErr) || coreErr.Code != core.CodeResourceUnavailable {
		t.Fatalf("Preflight = %+v, %v; want core.Error %s", report, err, core.CodeResourceUnavailable)
	}
}

func TestSkipVerifyWarns(t *testing.T) {
	pg := startPostgres(t)
	cfg := preflightResourceConfig(pg)
	cfg.TLS = core.TLSInsecureSkipVerify
	cfg.TLSCA = nil
	report := mustSQLPreflight(t, pg, cfg)
	if !report.Passed || !reportCheck(t, report, checkTLSName).Passed || len(report.Warnings) != 1 {
		t.Fatalf("report = %+v; want passed with exactly one warning", report)
	}
}

func TestRefusesDblink(t *testing.T) {
	pg := startPostgres(t)
	preflightFixture(t, pg, "CREATE EXTENSION dblink")
	stores := map[string]core.ApprovalVerifier{
		"nil approvals": nil,
		"empty store":   approval.NewStore(filepath.Join(t.TempDir(), approval.DirName), bytes.Repeat([]byte{0x51}, 32)),
	}
	for name, approvals := range stores {
		cfg := preflightResourceConfig(pg)
		cfg.Approvals = approvals
		report := mustSQLPreflight(t, pg, cfg)
		if report.Passed || reportCheck(t, report, checkRiskyObjects).Passed {
			t.Fatalf("%s: report = %+v; want risky_objects refused", name, report)
		}
		want := "resource main has risky objects dblink; to approve, run on the host: podman exec -it sag sag extension approve main"
		if report.Hint != want {
			t.Fatalf("%s: hint = %q; want %q", name, report.Hint, want)
		}
	}
}

func TestApprovalAccepts(t *testing.T) {
	pg := startPostgres(t)
	preflightFixture(t, pg, "CREATE EXTENSION dblink")
	store := approval.NewStore(filepath.Join(t.TempDir(), approval.DirName), bytes.Repeat([]byte{0x52}, 32))
	cfg := preflightResourceConfig(pg)
	cfg.Approvals = store
	ctx, cancel := context.WithTimeout(context.Background(), preflightTestTimeout)
	defer cancel()
	target, objects, err := Adapter{}.Inspect(ctx, cfg, preflightCred(pg))
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	fp, err := approval.Fingerprint(target, objects)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Save(cfg.Name, fp, objects, time.Now()); err != nil {
		t.Fatalf("Save: %v", err)
	}
	report := mustSQLPreflight(t, pg, cfg)
	if !report.Passed || !reportCheck(t, report, checkRiskyObjects).Passed {
		t.Fatalf("report = %+v; want passed with a matching approval", report)
	}

	preflightFixture(t, pg, "CREATE EXTENSION file_fdw")
	report = mustSQLPreflight(t, pg, cfg)
	if report.Passed || reportCheck(t, report, checkRiskyObjects).Passed || !strings.Contains(report.Hint, "file_fdw") {
		t.Fatalf("report = %+v; want refused with file_fdw in the hint after a new risky extension", report)
	}
}

func TestPreflightIgnoresRoleSearchPath(t *testing.T) {
	pg := startPostgres(t)
	preflightFixture(t, pg,
		"CREATE EXTENSION dblink",
		"CREATE SCHEMA rev_evil",
		"CREATE FUNCTION rev_evil.never(name, name) RETURNS boolean LANGUAGE sql IMMUTABLE AS 'SELECT false'",
		"CREATE OPERATOR rev_evil.= (LEFTARG = name, RIGHTARG = name, FUNCTION = rev_evil.never)",
		"ALTER ROLE CURRENT_USER SET search_path = rev_evil, pg_catalog, public",
	)
	report := mustSQLPreflight(t, pg, preflightResourceConfig(pg))
	if report.Passed || reportCheck(t, report, checkRiskyObjects).Passed || !strings.Contains(report.Hint, "dblink") {
		t.Fatalf("report = %+v; want dblink refused with a role search_path that shadows =", report)
	}
}

func TestReportsCitextOperator(t *testing.T) {
	pg := startPostgres(t)
	preflightFixture(t, pg, "CREATE EXTENSION citext")
	report := mustSQLPreflight(t, pg, preflightResourceConfig(pg))
	c := reportCheck(t, report, checkOperatorNames)
	if !report.Passed || !c.Passed || !slices.Contains(strings.Fields(c.Detail), "~~*") {
		t.Fatalf("report = %+v; want passed with the citext operator ~~* in operator_names", report)
	}
}

type readWriteBegin struct {
	*dialect
}

func (readWriteBegin) RequestBegin(core.Limits) []string {
	return []string{"BEGIN TRANSACTION READ WRITE"}
}

func TestReadOnlyModeCheck(t *testing.T) {
	pg := startPostgres(t)
	start := time.Now()
	report := mustSQLPreflight(t, pg, preflightResourceConfig(pg))
	if !report.Passed || !reportCheck(t, report, checkReadOnlyMode).Passed {
		t.Fatalf("report = %+v; want read_only_mode passed after RequestBegin", report)
	}
	ctx, cancel := context.WithTimeout(context.Background(), preflightTestTimeout)
	defer cancel()
	logs, err := pg.Logs(ctx, start)
	if err != nil {
		t.Fatalf("Logs: %v", err)
	}
	begin := strings.Index(logs, "BEGIN TRANSACTION READ ONLY")
	ssl := strings.Index(logs, "pg_stat_ssl")
	operators := strings.Index(logs, "pg_operator")
	rollback := strings.LastIndex(logs, "ROLLBACK")
	if begin < 0 || ssl < begin || operators < ssl || rollback < operators || strings.Contains(logs, "COMMIT") {
		t.Fatalf("server log lacks BEGIN READ ONLY, preflight reads, ROLLBACK in order, or has COMMIT:\n%s", logs)
	}

	d, p, err := openSQL(ctx, preflightResourceConfig(pg), preflightCred(pg))
	if err != nil {
		t.Fatalf("openSQL: %v", err)
	}
	defer p.Close()
	rw := readWriteBegin{d}
	err = sql.RunReadOnly(ctx, rw, p, core.Limits{Timeout: sql.CatalogTimeout}, func(ctx context.Context, r sql.CatalogReader) error {
		var err error
		report, err = rw.Preflight(ctx, r)
		return err
	})
	if err != nil {
		t.Fatalf("Preflight in a read-write transaction: %v", err)
	}
	if report.Passed || reportCheck(t, report, checkReadOnlyMode).Passed {
		t.Fatalf("report = %+v; want read_only_mode refused in a read-write transaction", report)
	}
}
