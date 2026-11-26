//go:build devxe2e

package devx_test

import (
	"context"
	"os"
	"os/exec"
	"syscall"
	"testing"

	"github.com/kanthorlabs/sag/e2e/devx/dxh"
)

func testDXAC04BootstrapNoTerminal(t *testing.T, ctx context.Context, groupDir, binDir string) {
	t.Helper()
	home := os.Getenv("HOME")
	cfg := cfgDir(home)

	env, err := dxh.ChildEnv(os.Environ(), groupDir)
	if err != nil {
		t.Fatalf("DXAC04: child env: %v", err)
	}
	env = append(env, "PATH="+dxh.TestPATH(binDir))

	cmd := exec.CommandContext(ctx, "sag-devxtest", "bootstrap")
	cmd.Env = env
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	out, err := cmd.CombinedOutput()
	code := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			code = exitErr.ExitCode()
		} else {
			t.Fatalf("DXAC04: run sag-devxtest bootstrap: %v", err)
		}
	}

	if code != 3 {
		t.Fatalf("DXAC04: exit code = %d; want 3; output: %s", code, out)
	}

	if _, statErr := os.Stat(cfg); statErr == nil {
		t.Fatalf("DXAC04: cfg dir %s exists after exit 3; want absent", cfg)
	}
}
