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
	deleteTests["DXAC18_DeleteNoTerminal"] = func(t *testing.T, ctx context.Context) {
		DXAC18_DeleteNoTerminal(t, ctx)
	}
}

func DXAC18_DeleteNoTerminal(t *testing.T, ctx context.Context) {
	t.Helper()
	dxh.EnsureCanary(t)

	uid := fmt.Sprintf("%d", os.Getuid())
	if err := dxh.WaitLaunchAgentIdle(ctx, dxh.NewRunner(), uid); err != nil {
		t.Fatalf("DXAC18: wait launch agent idle: %v", err)
	}

	code, combined := runSAGSetsid(t, ctx, nil, "credential", "delete", "canary")
	if code != 3 {
		t.Fatalf("DXAC18: setsid delete: exit %d; want 3; output: %s", code, combined)
	}

	if !secretExists(ctx, t, "sag-res-canary.conn") {
		t.Fatalf("DXAC18: canary conn secret missing after refused delete")
	}

	r := dxh.NewRunner()
	if _, err := r.Output(ctx, "podman", "exec", "sag", "sag", "resource", "test", "canary"); err != nil {
		t.Fatalf("DXAC18: resource test canary: %v", err)
	}
}

func deleteViaStartTTY(t *testing.T, ctx context.Context, name string) (int, *dxh.TTY) {
	t.Helper()
	bin, err := dxh.TestBinary()
	if err != nil {
		t.Fatalf("deleteViaStartTTY: TestBinary: %v", err)
	}
	groupDir := t.TempDir()
	binDir := t.TempDir()
	tty, err := dxh.StartTTY(ctx, groupDir, binDir, bin, "credential", "delete", name)
	if err != nil {
		t.Fatalf("deleteViaStartTTY: StartTTY: %v", err)
	}
	if err := tty.Expect(ctx, "type the resource name"); err != nil {
		t.Fatalf("deleteViaStartTTY: expect prompt: %v\noutput: %s", err, tty.RawOutput())
	}
	if err := tty.Send(name + "\n"); err != nil {
		t.Fatalf("deleteViaStartTTY: send name: %v", err)
	}
	code, err := tty.Wait(ctx)
	if err != nil {
		t.Fatalf("deleteViaStartTTY: wait: %v", err)
	}
	return code, tty
}
