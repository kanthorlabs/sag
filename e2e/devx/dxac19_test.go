//go:build devxe2e

package devx_test

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/kanthorlabs/sag/e2e/devx/dxh"
)

func init() {
	deleteTests["DXAC19_DeleteConfirm"] = func(t *testing.T, ctx context.Context) {
		DXAC19_DeleteConfirm(t, ctx)
	}
}

func DXAC19_DeleteConfirm(t *testing.T, ctx context.Context) {
	t.Helper()
	dxh.EnsureCanary(t)

	uid := fmt.Sprintf("%d", os.Getuid())
	if err := dxh.WaitLaunchAgentIdle(ctx, dxh.NewRunner(), uid); err != nil {
		t.Fatalf("DXAC19: wait launch agent idle: %v", err)
	}

	t.Run("yes_flag_exit2", func(t *testing.T) {
		code, combined := runSAGSetsid(t, ctx, nil, "credential", "delete", "--yes", "canary")
		if code != 2 {
			t.Fatalf("DXAC19: --yes: exit %d; want 2; output: %s", code, combined)
		}
		if !secretExists(ctx, t, "sag-res-canary.conn") {
			t.Fatal("DXAC19: canary conn secret missing after refused --yes delete")
		}
	})

	t.Run("ac_order_exit2", func(t *testing.T) {
		code, combined := runSAGSetsid(t, ctx, nil, "credential", "delete", "canary", "--yes")
		if code != 2 {
			t.Fatalf("DXAC19: canary --yes: exit %d; want 2; output: %s", code, combined)
		}
		if !secretExists(ctx, t, "sag-res-canary.conn") {
			t.Fatal("DXAC19: canary conn secret missing after refused canary --yes delete")
		}
	})

	t.Run("wrong_name_exit3", func(t *testing.T) {
		bin, err := dxh.TestBinary()
		if err != nil {
			t.Fatalf("DXAC19: TestBinary: %v", err)
		}
		groupDir := t.TempDir()
		binDir := t.TempDir()
		tty, err := dxh.StartTTY(ctx, groupDir, binDir, bin, "credential", "delete", "canary")
		if err != nil {
			t.Fatalf("DXAC19: StartTTY: %v", err)
		}
		if err := tty.Expect(ctx, "type the resource name"); err != nil {
			t.Fatalf("DXAC19: expect prompt: %v\noutput: %s", err, tty.RawOutput())
		}
		if err := tty.Send("WRONG-NAME\n"); err != nil {
			t.Fatalf("DXAC19: send wrong name: %v", err)
		}
		code, err := tty.Wait(ctx)
		if err != nil {
			t.Fatalf("DXAC19: wait: %v", err)
		}
		if code != 3 {
			t.Fatalf("DXAC19: wrong name: exit %d; want 3; output: %s", code, tty.RawOutput())
		}
		if !secretExists(ctx, t, "sag-res-canary.conn") {
			t.Fatal("DXAC19: canary conn secret missing after wrong name delete")
		}
	})
}
