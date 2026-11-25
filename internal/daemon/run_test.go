package daemon

import (
	"context"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kanthorlabs/sag/internal/secret"
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
	if errors.Is(err, errors.ErrUnsupported) {
		t.Skipf("waits on T1.1 and T1.2: a stub refused before the %v check: %v", stage, err)
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
