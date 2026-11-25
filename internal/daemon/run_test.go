package daemon

import (
	"context"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kanthorlabs/sag/internal/core"
	"github.com/kanthorlabs/sag/internal/secret"
	"github.com/kanthorlabs/sag/internal/sql"
)

const (
	testDSNPassword = "dsn-secret-password"
	testDSN         = "postgres://app:" + testDSNPassword + "@db.example.net:5432/app"
	testConfig      = `version: 1
server:
  tls_cert: secret:sag-tls-cert
  tls_key: secret:sag-tls-key
resources:
  main:
    kind: postgres
    connection: secret:sag-main
    agents: [skeleton]
`
)

type fixture struct {
	opts    Options
	log     *strings.Builder
	secrets map[string][]byte
}

func newFixture(t *testing.T, masterKey []byte) fixture {
	t.Helper()
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "sag.yaml")
	if err := os.WriteFile(cfgPath, []byte(testConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	secretsDir := filepath.Join(dir, "secrets")
	if err := os.Mkdir(secretsDir, 0o700); err != nil {
		t.Fatal(err)
	}
	f := fixture{
		log: &strings.Builder{},
		secrets: map[string][]byte{
			secret.MasterKeyName: []byte(base64.StdEncoding.EncodeToString(masterKey)),
			"sag-tls-cert":       []byte("cert-pem"),
			"sag-tls-key":        []byte("key-pem"),
			"sag-main":           []byte(testDSN),
		},
	}
	for name, value := range f.secrets {
		if err := os.WriteFile(filepath.Join(secretsDir, name), value, 0o400); err != nil {
			t.Fatal(err)
		}
	}
	f.opts = Options{
		ConfigPath: cfgPath,
		SecretsDir: secretsDir,
		SecretUID:  os.Getuid(),
		StateDir:   filepath.Join(dir, "state"),
		Version:    "test",
		Log:        f.log,
	}
	return f
}

func (f fixture) assertNoSecret(t *testing.T, err error) {
	t.Helper()
	for name, value := range f.secrets {
		for _, text := range []string{err.Error(), f.log.String()} {
			if strings.Contains(text, string(value)) {
				t.Fatalf("output holds the value of secret %s", name)
			}
		}
	}
	for _, text := range []string{err.Error(), f.log.String()} {
		if strings.Contains(text, testDSNPassword) {
			t.Fatal("output holds the DSN password")
		}
	}
}

func assertRefused(t *testing.T, err, stage error) {
	t.Helper()
	if err == nil {
		t.Fatal("Run returned nil; want a refusal")
	}
	if !errors.Is(err, stage) {
		t.Fatalf("Run error = %v; want stage %v", err, stage)
	}
}

func TestRefusesBadMasterKey(t *testing.T) {
	f := newFixture(t, make([]byte, 16))
	err := Run(context.Background(), f.opts)
	assertRefused(t, err, errMasterKey)
	f.assertNoSecret(t, err)
}

func TestRefusesSecretMode(t *testing.T) {
	f := newFixture(t, make([]byte, 32))
	if err := os.Chmod(filepath.Join(f.opts.SecretsDir, "sag-main"), 0o444); err != nil {
		t.Fatal(err)
	}
	err := Run(context.Background(), f.opts)
	assertRefused(t, err, errSecrets)
	f.assertNoSecret(t, err)
}

func TestConfigErrorHidesDSN(t *testing.T) {
	for name, text := range map[string]string{
		"dsn as kind":          strings.Replace(testConfig, "kind: postgres", "kind: "+testDSN, 1),
		"dsn as unknown key":   testConfig + "    \"" + testDSN + "\": 1\n",
		"dsn as resource name": strings.Replace(testConfig, "  main:", "  \""+testDSN+"\":", 1),
	} {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t, make([]byte, 32))
			if err := os.WriteFile(f.opts.ConfigPath, []byte(text), 0o600); err != nil {
				t.Fatal(err)
			}
			err := Run(context.Background(), f.opts)
			assertRefused(t, err, errConfig)
			f.assertNoSecret(t, err)
			for _, out := range []string{err.Error(), f.log.String()} {
				if strings.Contains(out, "db.example.net") {
					t.Fatalf("output holds the DSN host: %s", out)
				}
			}
		})
	}
}

type openOnlyAdapter struct{ fake *fakeAdapter }

func (a openOnlyAdapter) Kind() string { return "daemon-open-only" }

func (a openOnlyAdapter) Capabilities() []core.Capability { return a.fake.Capabilities() }

func (a openOnlyAdapter) Open(ctx context.Context, cfg core.ResourceConfig, cred secret.Credential) (core.Resource, error) {
	return a.fake.Open(ctx, cfg, cred)
}

func TestWrapDialectNilInProduct(t *testing.T) {
	ctx := context.Background()
	rc := core.ResourceConfig{Name: "main", Kind: fakeKind}
	cred := secret.NewCredential("sag-main", []byte(testDSN))

	fake := &fakeAdapter{}
	res, err := openResource(ctx, fake, rc, cred, nil)
	if err != nil {
		t.Fatal(err)
	}
	if fake.opened != 1 || fake.openedSQL != 0 || res != fake.resource {
		t.Fatalf("nil WrapDialect: Open %d, OpenSQL %d, resource %#v; want the adapter Open path", fake.opened, fake.openedSQL, res)
	}

	wrapCalls := 0
	wrap := func(d sql.Dialect) sql.Dialect {
		wrapCalls++
		return d
	}
	res, err = openResource(ctx, fake, rc, cred, wrap)
	if err != nil {
		t.Fatal(err)
	}
	if fake.opened != 1 || fake.openedSQL != 1 || wrapCalls != 1 || res == nil || res == core.Resource(fake.resource) {
		t.Fatalf("WrapDialect set: Open %d, OpenSQL %d, wrap %d, resource %#v; want the wrapped executor", fake.opened, fake.openedSQL, wrapCalls, res)
	}

	plain := openOnlyAdapter{fake: &fakeAdapter{}}
	res, err = openResource(ctx, plain, rc, cred, wrap)
	if err != nil {
		t.Fatal(err)
	}
	if plain.fake.opened != 1 || wrapCalls != 1 || res != plain.fake.resource {
		t.Fatal("WrapDialect set on an adapter without sql.Opener: want the adapter Open path")
	}
}
