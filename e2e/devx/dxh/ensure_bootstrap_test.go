package dxh

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

type unitRunner struct {
	out []byte
	err error
}

func (u *unitRunner) Output(_ context.Context, _ string, _ ...string) ([]byte, error) {
	return u.out, u.err
}

func TestEnsureBootstrappedSkipsWhenPresent(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	cfgDir := filepath.Join(tmpHome, ".config", "sag")
	if err := os.MkdirAll(cfgDir, 0o700); err != nil {
		t.Fatal(err)
	}

	runCalled := false
	oldRun := ensureBootstrapRunFn
	oldNew := newRunnerFn
	defer func() {
		ensureBootstrapRunFn = oldRun
		newRunnerFn = oldNew
	}()

	newRunnerFn = func() Runner {
		return &unitRunner{out: []byte("true\n")}
	}
	ensureBootstrapRunFn = func(_ context.Context, _ Runner) error {
		runCalled = true
		return nil
	}

	EnsureBootstrapped(t)

	if runCalled {
		t.Fatal("bootstrap run was called even though already present")
	}
}

func TestEnsureBootstrappedRunsWhenCfgAbsent(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	runCalled := false
	oldRun := ensureBootstrapRunFn
	oldNew := newRunnerFn
	defer func() {
		ensureBootstrapRunFn = oldRun
		newRunnerFn = oldNew
	}()

	newRunnerFn = func() Runner {
		return &unitRunner{out: []byte("true\n")}
	}
	ensureBootstrapRunFn = func(_ context.Context, _ Runner) error {
		runCalled = true
		return nil
	}

	EnsureBootstrapped(t)

	if !runCalled {
		t.Fatal("bootstrap run was not called even though cfg is absent")
	}
}

func TestEnsureBootstrappedRunsWhenContainerDown(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	cfgDir := filepath.Join(tmpHome, ".config", "sag")
	if err := os.MkdirAll(cfgDir, 0o700); err != nil {
		t.Fatal(err)
	}

	runCalled := false
	oldRun := ensureBootstrapRunFn
	oldNew := newRunnerFn
	defer func() {
		ensureBootstrapRunFn = oldRun
		newRunnerFn = oldNew
	}()

	newRunnerFn = func() Runner {
		return &unitRunner{out: []byte("false\n")}
	}
	ensureBootstrapRunFn = func(_ context.Context, _ Runner) error {
		runCalled = true
		return nil
	}

	EnsureBootstrapped(t)

	if !runCalled {
		t.Fatal("bootstrap run was not called even though container is not running")
	}
}
