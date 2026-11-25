//go:build integration

package postgres

import (
	"context"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/kanthorlabs/sag/internal/core"
	"github.com/kanthorlabs/sag/internal/secret"
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
	version := strings.Index(logs, "SELECT version()")
	rollback := strings.Index(logs, "ROLLBACK")
	if begin < 0 || version < begin || rollback < version {
		t.Fatalf("server log lacks BEGIN READ ONLY, SELECT version(), ROLLBACK in order:\n%s", logs)
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
	cfg := core.ResourceConfig{Name: "main", Kind: Kind, TLS: core.TLSVerifyFull, TLSCA: other.CertPEM, ConnectTimeout: 10 * time.Second}
	res := openResource(t, cfg, pg.HostDSN())
	if _, err := res.Preflight(context.Background()); err == nil {
		t.Fatal("Preflight passed with a CA that did not sign the server certificate")
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
	cfg := core.ResourceConfig{Name: "main", Kind: Kind, TLS: core.TLSVerifyFull, TLSCA: pg.CAPEM, ConnectTimeout: 10 * time.Second}
	res := openResource(t, cfg, u.String())
	_, err = res.Preflight(context.Background())
	if err == nil {
		t.Fatal("Preflight passed with a wrong password")
	}
	if strings.Contains(err.Error(), wrong) {
		t.Fatalf("error text leaks the password: %q", err.Error())
	}
}
