package dxh

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureCanarySkipsWhenPresent(t *testing.T) {
	dir := t.TempDir()
	sagYAML := filepath.Join(dir, "sag.yaml")
	if err := os.WriteFile(sagYAML, []byte("resources:\n  canary:\n    kind: fakedb\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	prev := ActiveCanaryParts
	t.Cleanup(func() { ActiveCanaryParts = prev })
	ActiveCanaryParts = CanaryParts{Password: "x", Host: "h", User: "u", Database: "d"}

	createCalled := false

	oldBootstrap := ensureCanaryBootstrapFn
	t.Cleanup(func() { ensureCanaryBootstrapFn = oldBootstrap })
	ensureCanaryBootstrapFn = func(t *testing.T) { t.Helper() }

	oldCreate := ensureCanaryCreateFn
	t.Cleanup(func() { ensureCanaryCreateFn = oldCreate })
	ensureCanaryCreateFn = func(_ context.Context, _ Runner, t *testing.T) error {
		createCalled = true
		return nil
	}

	oldRunner := newRunnerFn
	t.Cleanup(func() { newRunnerFn = oldRunner })
	newRunnerFn = func() Runner { return fakeCanaryRunner{} }

	oldEnv := os.Getenv("HOME")
	t.Setenv("HOME", dir)
	defer func() { os.Setenv("HOME", oldEnv) }()

	if err := os.MkdirAll(filepath.Join(dir, ".config", "sag"), 0o755); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(dir, ".config", "sag", "sag.yaml")
	if err := os.WriteFile(dst, []byte("resources:\n  canary:\n    kind: fakedb\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	EnsureCanary(t)

	if createCalled {
		t.Error("EnsureCanary called create when canary already present")
	}
}

type fakeCanaryRunner struct{}

func (fakeCanaryRunner) Output(_ context.Context, _ string, _ ...string) ([]byte, error) {
	return []byte("true"), nil
}
