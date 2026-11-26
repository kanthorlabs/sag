//go:build devxe2e

package devx_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/kanthorlabs/sag/e2e/devx/dxh"
)

func DXAC28_ServiceStopStart(t *testing.T, ctx context.Context) {
	t.Helper()
	dxh.EnsureBootstrapped(t)

	home := os.Getenv("HOME")
	caPath := filepath.Join(dxh.CfgDir(home), "ca.pem")

	out, err := runSAG(ctx, "service", "stop")
	if err != nil {
		t.Fatalf("service stop: %v\n%s", err, out)
	}

	if containerRunning(ctx) {
		t.Fatal("container still running after service stop")
	}

	if !healthzFails(ctx, caPath) {
		t.Fatal("/healthz returned 200 after service stop")
	}

	out, err = runSAG(ctx, "service", "start")
	if err != nil {
		t.Fatalf("service start: %v\n%s", err, out)
	}

	if err := healthzOK(ctx, caPath); err != nil {
		t.Fatalf("/healthz after service start: %v", err)
	}
}
