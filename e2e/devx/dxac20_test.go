//go:build devxe2e

package devx_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kanthorlabs/sag/e2e/devx/dxh"
)

func init() {
	deleteTests["DXAC20_Delete"] = func(t *testing.T, ctx context.Context) {
		DXAC20_Delete(t, ctx)
	}
}

func DXAC20_Delete(t *testing.T, ctx context.Context) {
	t.Helper()
	dxh.EnsureCanary(t)

	uid := fmt.Sprintf("%d", os.Getuid())
	if err := dxh.WaitLaunchAgentIdle(ctx, dxh.NewRunner(), uid); err != nil {
		t.Fatalf("DXAC20: wait launch agent idle: %v", err)
	}

	code, tty := deleteViaStartTTY(t, ctx, "canary")
	if code != 0 {
		t.Fatalf("DXAC20: delete canary: exit %d; want 0; output: %s", code, tty.RawOutput())
	}

	if secretExists(ctx, t, "sag-res-canary.conn") {
		t.Fatalf("DXAC20: canary conn secret still exists after delete")
	}

	home := os.Getenv("HOME")
	sagYAML := filepath.Join(dxh.CfgDir(home), "sag.yaml")
	data, err := os.ReadFile(sagYAML)
	if err != nil {
		t.Fatalf("DXAC20: read sag.yaml: %v", err)
	}
	if strings.Contains(string(data), "canary:") {
		t.Fatalf("DXAC20: canary still in sag.yaml")
	}

	r := dxh.NewRunner()
	if _, err := r.Output(ctx, "podman", "cp", "sag:/var/lib/sag/approvals/canary.revoked.json", "-"); err != nil {
		t.Fatalf("DXAC20: tombstone missing: %v", err)
	}

	out, err := r.Output(ctx, "podman", "secret", "ls", "--filter", "name=sag-res-canary", "--format", "{{.Name}}")
	if err != nil {
		t.Fatalf("DXAC20: secret ls: %v", err)
	}
	if strings.Contains(string(out), "sag-res-canary") {
		t.Fatalf("DXAC20: canary secret still exists: %s", out)
	}
}
