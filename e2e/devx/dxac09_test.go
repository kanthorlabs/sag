//go:build devxe2e

package devx_test

import (
	"context"
	"strings"
	"testing"

	"github.com/kanthorlabs/sag/e2e/devx/dxh"
)

func init() {
	credentialTests["DXAC09_CreateCanary"] = func(t *testing.T, ctx context.Context) {
		DXAC09_CreateCanary(t, ctx)
	}
}

func DXAC09_CreateCanary(t *testing.T, ctx context.Context) {
	t.Helper()
	dxh.EnsureCanary(t)

	out, err := runSAG(t, ctx, "credential", "list")
	if err != nil {
		t.Fatalf("DXAC09: credential list: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "canary") {
		t.Fatalf("DXAC09: credential list does not contain canary:\n%s", out)
	}
}
