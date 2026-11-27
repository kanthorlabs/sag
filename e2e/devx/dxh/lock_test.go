package dxh_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kanthorlabs/sag/e2e/devx/dxh"
)

func TestCheckRunLockHeldByLiveProcess(t *testing.T) {
	dir := t.TempDir()
	holdRunLock(t, dir)
	if err := dxh.CheckRunLock(dir); err != nil {
		t.Fatal(err)
	}
}

func TestCheckRunLockAbsent(t *testing.T) {
	t.Setenv("DEVX_RUN_TOKEN", testRunToken)
	if err := dxh.CheckRunLock(t.TempDir()); err == nil {
		t.Fatal("CheckRunLock passed without a lock")
	}
}

func TestCheckRunLockDeadOwner(t *testing.T) {
	dir := t.TempDir()
	lock := filepath.Join(dir, "claims", "devx-e2e.lock")
	if err := os.MkdirAll(lock, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DEVX_RUN_TOKEN", testRunToken)
	if err := os.WriteFile(filepath.Join(lock, "owner"), []byte("999999 "+testRunToken), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := dxh.CheckRunLock(dir); err == nil {
		t.Fatal("CheckRunLock passed with a dead owner")
	}
}

func TestCheckRunLockNoToken(t *testing.T) {
	dir := t.TempDir()
	lock := filepath.Join(dir, "claims", "devx-e2e.lock")
	if err := os.MkdirAll(lock, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(lock, "owner"), []byte("999999 some-token"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := dxh.CheckRunLock(dir); err == nil {
		t.Fatal("CheckRunLock passed with no DEVX_RUN_TOKEN set")
	}
}

func TestCheckRunLockWrongToken(t *testing.T) {
	dir := t.TempDir()
	holdRunLock(t, dir)
	t.Setenv("DEVX_RUN_TOKEN", "wrong-token-not-matching")
	if err := dxh.CheckRunLock(dir); err == nil {
		t.Fatal("CheckRunLock passed with a mismatched token")
	}
}
