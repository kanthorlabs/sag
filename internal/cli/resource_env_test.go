package cli

import (
	"bytes"
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"

	"github.com/kanthorlabs/sag/internal/core"
)

func writeSecret(t *testing.T, dir, name string, data []byte) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), data, 0o400); err != nil {
		t.Fatal(err)
	}
}

func TestLoadResourceBuildsConfig(t *testing.T) {
	secrets := t.TempDir()
	writeSecret(t, secrets, "sag-partner-ledger", []byte("postgres://u:p@db.example.net:5432/ledger"))
	writeSecret(t, secrets, "sag-master-key", []byte(base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{7}, 32))))
	state := t.TempDir()
	setConfigCheckPaths(t, Paths{Config: configFixture("ac01-valid"), Secrets: secrets, SecretUID: os.Getuid(), State: state})

	target, err := loadResource("partner-ledger")
	if err != nil {
		t.Fatalf("loadResource: %v", err)
	}
	if target.Adapter == nil || target.Adapter.Kind() != "postgres" {
		t.Fatalf("Adapter = %v; want postgres", target.Adapter)
	}
	if target.Config.Name != "partner-ledger" || target.Config.TLS != "insecure-skip-verify" {
		t.Fatalf("Config = %+v", target.Config)
	}
	if target.Config.LockTimeout == 0 || target.Config.IdleInTransactionSessionTimeout == 0 {
		t.Fatalf("Config lacks lock and idle timeouts: %+v", target.Config)
	}
	if target.Approvals == nil || target.Config.Approvals != core.ApprovalVerifier(target.Approvals) {
		t.Fatalf("Approvals = %v, Config.Approvals = %v; want the same store", target.Approvals, target.Config.Approvals)
	}
	if string(target.Cred.Reveal()) != "postgres://u:p@db.example.net:5432/ledger" {
		t.Fatal("Cred does not hold the connection secret")
	}
}

func TestLoadResourceUnknownName(t *testing.T) {
	setConfigCheckPaths(t, Paths{Config: configFixture("ac01-valid"), Secrets: t.TempDir(), SecretUID: os.Getuid(), State: t.TempDir()})
	if _, err := loadResource("absent"); err == nil {
		t.Fatal("loadResource(absent) returned no error")
	}
}
