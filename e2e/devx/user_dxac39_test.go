//go:build devxe2e

package devx_test

import (
	"context"
	"testing"
)

func init() {
	userHarnessTests["DXAC39_DefaultPath"] = func(t *testing.T, ctx context.Context) {
		DXAC39_DefaultPath(t, ctx)
	}
}

func DXAC39_DefaultPath(t *testing.T, ctx context.Context) {
	t.Helper()
	t.Skip("DXAC39_DefaultPath: requires D9.1 real user harness; skip at Layer 6")
}
