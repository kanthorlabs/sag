//go:build devxe2e

package dxh

import (
	"context"
	"testing"
)

func TestEnsureLocalStackSkipsWhenPresent(t *testing.T) {
	oldBootstrap := ensureLocalStackBootstrapFn
	oldNew := newRunnerFn
	defer func() {
		ensureLocalStackBootstrapFn = oldBootstrap
		newRunnerFn = oldNew
	}()

	ensureLocalStackBootstrapFn = func(t *testing.T) {}

	newRunnerFn = func() Runner {
		return &unitRunner{out: []byte("true\n")}
	}

	running := localStackRunning(context.Background(), &unitRunner{out: []byte("true\n")})
	if !running {
		t.Fatal("localStackRunning must return true when container reports Running=true")
	}
}
