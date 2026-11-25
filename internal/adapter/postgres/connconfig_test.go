package postgres

import (
	"context"
	"crypto/tls"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/kanthorlabs/sag/internal/core"
	"github.com/kanthorlabs/sag/internal/secret"
)

const testPassword = "s3cret-Pw_9"

func verifyFull() core.ResourceConfig {
	return core.ResourceConfig{Name: "main", Kind: Kind, TLS: core.TLSVerifyFull, ConnectTimeout: 5 * time.Second}
}

func cred(dsn string) secret.Credential {
	return secret.NewCredential("sag-main", []byte(dsn))
}

func mustConnConfig(t *testing.T, cfg core.ResourceConfig, dsn string) *pgx.ConnConfig {
	t.Helper()
	cc, err := connConfig(cfg, cred(dsn))
	if err != nil {
		t.Fatalf("connConfig(%q) error: %v", dsn, err)
	}
	return cc
}

func assertNoSecret(t *testing.T, err error) {
	t.Helper()
	if strings.Contains(err.Error(), testPassword) || strings.Contains(err.Error(), "postgres://") {
		t.Fatalf("error text leaks the DSN or the password: %q", err.Error())
	}
}

func TestBuildsFromSecretFields(t *testing.T) {
	cc := mustConnConfig(t, verifyFull(), "postgres://app:"+testPassword+"@db.example.net:6543/orders")
	if cc.Host != "db.example.net" || cc.Port != 6543 || cc.Database != "orders" || cc.User != "app" || cc.Password != testPassword {
		t.Fatalf("config = host %q port %d db %q user %q", cc.Host, cc.Port, cc.Database, cc.User)
	}
	if cc.ConnectTimeout != 5*time.Second {
		t.Fatalf("ConnectTimeout = %v; want 5s", cc.ConnectTimeout)
	}
	if len(cc.RuntimeParams) != 0 {
		t.Fatalf("RuntimeParams = %v; want empty", cc.RuntimeParams)
	}
}

func TestDefaultPort(t *testing.T) {
	cc := mustConnConfig(t, verifyFull(), "postgresql://app:"+testPassword+"@db.example.net/orders")
	if cc.Port != 5432 {
		t.Fatalf("Port = %d; want 5432", cc.Port)
	}
}

func TestDecodesPercentEncodedPassword(t *testing.T) {
	cc := mustConnConfig(t, verifyFull(), "postgres://app:p%40ss%2Fw@db.example.net:5432/orders")
	if cc.Password != "p@ss/w" {
		t.Fatalf("Password not decoded")
	}
}

func TestRejectsQueryParams(t *testing.T) {
	for _, dsn := range []string{
		"postgres://app:" + testPassword + "@db.example.net:5432/orders?sslmode=disable",
		"postgres://app:" + testPassword + "@db.example.net:5432/orders?default_query_exec_mode=simple_protocol",
		"postgres://app:" + testPassword + "@db.example.net:5432/orders?target_session_attrs=any",
		"postgres://app:" + testPassword + "@db.example.net:5432/orders?",
		"postgres://app:" + testPassword + "@db.example.net:5432/orders#frag",
	} {
		_, err := connConfig(verifyFull(), cred(dsn))
		if err == nil {
			t.Fatalf("connConfig accepted a DSN with query parameters or fragment")
		}
		assertNoSecret(t, err)
	}
}

func TestRejectsMalformedDSN(t *testing.T) {
	for _, dsn := range []string{
		"host=db.example.net user=app password=" + testPassword + " dbname=orders",
		"mysql://app:" + testPassword + "@db.example.net:5432/orders",
		"postgres://app:" + testPassword + "@:5432/orders",
		"postgres://:" + testPassword + "@db.example.net:5432/orders",
		"postgres://app:" + testPassword + "@db.example.net:5432/",
		"postgres://app:" + testPassword + "@db.example.net:5432/orders/extra",
		"postgres://app:" + testPassword + "@db.example.net:0/orders",
		"postgres://app:" + testPassword + "@db.example.net:70000/orders",
		"postgres://app:" + testPassword + "@db.example.net:5432/orders\x7f%zz",
		"",
	} {
		_, err := connConfig(verifyFull(), cred(dsn))
		if err == nil {
			t.Fatalf("connConfig accepted a malformed DSN")
		}
		assertNoSecret(t, err)
	}
}

func TestExecModeExec(t *testing.T) {
	cc := mustConnConfig(t, verifyFull(), "postgres://app:"+testPassword+"@db.example.net:5432/orders")
	if cc.DefaultQueryExecMode != pgx.QueryExecModeExec {
		t.Fatalf("DefaultQueryExecMode = %v; want QueryExecModeExec", cc.DefaultQueryExecMode)
	}
}

func TestNoFallbacks(t *testing.T) {
	cc := mustConnConfig(t, verifyFull(), "postgres://app:"+testPassword+"@db.example.net:5432/orders")
	if cc.Fallbacks != nil {
		t.Fatalf("Fallbacks = %v; want nil", cc.Fallbacks)
	}
	for _, dsn := range []string{
		"postgres://app:" + testPassword + "@db1.example.net:5432,db2.example.net:5432/orders",
		"postgres://app:" + testPassword + "@db1.example.net,db2.example.net/orders",
	} {
		_, err := connConfig(verifyFull(), cred(dsn))
		if err == nil {
			t.Fatalf("connConfig accepted a multi-host DSN")
		}
		assertNoSecret(t, err)
	}
}

func TestVerifyFullDefault(t *testing.T) {
	for _, mode := range []core.TLSMode{core.TLSVerifyFull, ""} {
		cfg := verifyFull()
		cfg.TLS = mode
		cc := mustConnConfig(t, cfg, "postgres://app:"+testPassword+"@db.example.net:5432/orders")
		if cc.TLSConfig == nil || cc.TLSConfig.InsecureSkipVerify || cc.TLSConfig.ServerName != "db.example.net" {
			t.Fatalf("mode %q: TLSConfig = %+v; want verify-full for db.example.net", mode, cc.TLSConfig)
		}
	}
}

func TestTLSCAAddsRoots(t *testing.T) {
	cfg := verifyFull()
	cfg.TLSCA = []byte("not a pem bundle")
	_, err := connConfig(cfg, cred("postgres://app:"+testPassword+"@db.example.net:5432/orders"))
	if err == nil {
		t.Fatal("connConfig accepted a tls_ca without a certificate")
	}
	assertNoSecret(t, err)
}

func TestTLSModes(t *testing.T) {
	dsn := "postgres://app:" + testPassword + "@db.example.net:5432/orders"
	cfg := verifyFull()
	cfg.TLS = core.TLSInsecureSkipVerify
	if cc := mustConnConfig(t, cfg, dsn); cc.TLSConfig == nil || !cc.TLSConfig.InsecureSkipVerify {
		t.Fatalf("insecure-skip-verify: TLSConfig = %+v", cc.TLSConfig)
	}
	cfg.TLS = core.TLSDisable
	if cc := mustConnConfig(t, cfg, dsn); cc.TLSConfig != nil {
		t.Fatalf("disable: TLSConfig = %+v; want nil", cc.TLSConfig)
	}
	cfg.TLS = "prefer"
	if _, err := connConfig(cfg, cred(dsn)); err == nil {
		t.Fatal("connConfig accepted an unknown TLS mode")
	}
}

func TestLoopbackIPLiteralOnly(t *testing.T) {
	for _, host := range []string{"127.0.0.1", "127.10.20.30", "[::1]"} {
		cc := mustConnConfig(t, verifyFull(), "postgres://app:"+testPassword+"@"+host+":5432/orders")
		if cc.TLSConfig != nil {
			t.Fatalf("host %s: TLSConfig = %+v; want nil", host, cc.TLSConfig)
		}
	}
	for _, host := range []string{"localhost", "127.0.0.1.nip.io", "10.0.0.1", "[2001:db8::1]"} {
		cc := mustConnConfig(t, verifyFull(), "postgres://app:"+testPassword+"@"+host+":5432/orders")
		if cc.TLSConfig == nil || cc.TLSConfig.InsecureSkipVerify {
			t.Fatalf("host %s: TLSConfig = %+v; want verify-full", host, cc.TLSConfig)
		}
		want := strings.Trim(host, "[]")
		if cc.TLSConfig.ServerName != want {
			t.Fatalf("host %s: ServerName = %q", host, cc.TLSConfig.ServerName)
		}
		if cc.TLSConfig.MinVersion < tls.VersionTLS12 {
			t.Fatalf("host %s: MinVersion = %x", host, cc.TLSConfig.MinVersion)
		}
	}
}

func TestLoopbackExemptionOnlyUnderVerifyFull(t *testing.T) {
	for _, host := range []string{"127.0.0.1", "[::1]"} {
		dsn := "postgres://u:" + testPassword + "@" + host + ":5432/orders"
		cfg := verifyFull()
		cfg.TLS = ""
		if cc := mustConnConfig(t, cfg, dsn); cc.TLSConfig != nil {
			t.Fatalf("host %s, tls absent: TLSConfig = %+v; want nil", host, cc.TLSConfig)
		}
		cfg.TLS = core.TLSInsecureSkipVerify
		if cc := mustConnConfig(t, cfg, dsn); cc.TLSConfig == nil || !cc.TLSConfig.InsecureSkipVerify {
			t.Fatalf("host %s, insecure-skip-verify: TLSConfig = %+v; want TLS without verification", host, cc.TLSConfig)
		}
		cfg.TLS = core.TLSDisable
		if cc := mustConnConfig(t, cfg, dsn); cc.TLSConfig != nil {
			t.Fatalf("host %s, disable: TLSConfig = %+v; want nil", host, cc.TLSConfig)
		}
	}
}

func TestIgnoresPGEnvironment(t *testing.T) {
	t.Setenv("PGHOST", "evil.example.net")
	t.Setenv("PGPORT", "1")
	t.Setenv("PGSSLMODE", "disable")
	t.Setenv("PGAPPNAME", "evil")
	t.Setenv("PGTARGETSESSIONATTRS", "any")
	t.Setenv("PGOPTIONS", "-c default_transaction_read_only=off")
	cc := mustConnConfig(t, verifyFull(), "postgres://app:"+testPassword+"@db.example.net:5432/orders")
	if cc.Host != "db.example.net" || cc.Port != 5432 || cc.TLSConfig == nil || cc.Fallbacks != nil || len(cc.RuntimeParams) != 0 {
		t.Fatalf("PG environment leaked into the config: host %q port %d params %v", cc.Host, cc.Port, cc.RuntimeParams)
	}
}

func TestConnectErrorOmitsPassword(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	l.Close()
	cfg := verifyFull()
	cfg.PoolSize = 1
	res, err := Adapter{}.Open(context.Background(), cfg, cred("postgres://app:"+testPassword+"@"+addr+"/orders"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer res.Close()
	_, err = res.Preflight(context.Background())
	if err == nil {
		t.Fatal("Preflight succeeded against a closed port")
	}
	assertNoSecret(t, err)
}

func TestOpenErrorOmitsSecret(t *testing.T) {
	_, err := Adapter{}.Open(context.Background(), verifyFull(), cred("postgres://app:"+testPassword+"@db.example.net:5432/orders?sslmode=disable"))
	if err == nil {
		t.Fatal("Open accepted a DSN with query parameters")
	}
	assertNoSecret(t, err)
}

func TestRegisteredKind(t *testing.T) {
	a, ok := core.Lookup(Kind)
	if !ok || a.Kind() != "postgres" {
		t.Fatalf("Lookup(postgres) = %v, %v", a, ok)
	}
}
