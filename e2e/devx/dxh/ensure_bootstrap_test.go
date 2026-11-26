//go:build devxe2e

package dxh

import (
	"context"
	"testing"
)

func TestEnsureBootstrappedSkipsWhenPresent(t *testing.T) {
	oldCheck := ensureBootstrapCheckFn
	oldRun := ensureBootstrapRunFn
	defer func() {
		ensureBootstrapCheckFn = oldCheck
		ensureBootstrapRunFn = oldRun
	}()

	ensureBootstrapCheckFn = func(_ context.Context, _ Runner, _ string) bool {
		return true
	}

	runCalled := false
	ensureBootstrapRunFn = func(_ context.Context, _ Runner) error {
		runCalled = true
		return nil
	}

	EnsureBootstrapped(t)

	if runCalled {
		t.Fatal("bootstrap run was called even though already present")
	}
}
