//go:build devxe2e

package devx_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"

	"github.com/kanthorlabs/sag/e2e/devx/dxh"
)

func testDXAC36_HostGuards(t *testing.T, ctx context.Context, groupDir, binDir string) {
	t.Run("DXAC36_HostGuards", func(t *testing.T) {
		sagDevxtest, err := dxh.TestBinary()
		if err != nil {
			t.Fatalf("test binary: %v", err)
		}

		gr := dxh.NewGroupRunner(groupDir, binDir)

		internalCmds := [][]string{
			{sagDevxtest, "serve"},
			{sagDevxtest, "credential", "check", "dummy"},
			{sagDevxtest, "audit", "read"},
			{sagDevxtest, "agent", "issue", "a"},
		}
		for _, args := range internalCmds {
			_, err := gr.Output(ctx, args[0], args[1:]...)
			var exitErr *exec.ExitError
			if !errors.As(err, &exitErr) {
				t.Errorf("%v: want exit error, got %v", args[1:], err)
				continue
			}
			if exitErr.ExitCode() != 2 {
				t.Errorf("%v: exit %d; want 2", args[1:], exitErr.ExitCode())
			}
			if !strings.Contains(string(exitErr.Stderr), "runs only in the container") {
				t.Errorf("%v: stderr %q does not say 'runs only in the container'", args[1:], exitErr.Stderr)
			}
		}

		home := os.Getenv("HOME")
		cfgDir := filepath.Join(home, ".config", "sag")
		if err := os.MkdirAll(cfgDir, 0o700); err != nil {
			t.Fatalf("MkdirAll cfg dir: %v", err)
		}
		lockPath := filepath.Join(cfgDir, "lock")
		f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
		if err != nil {
			t.Fatalf("open lock file: %v", err)
		}
		defer f.Close()
		if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
			t.Fatalf("hold lock: %v", err)
		}
		defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)

		_, err = gr.Output(ctx, sagDevxtest, "bootstrap", "--autostart", "no")
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			t.Fatalf("bootstrap with lock held: want exit error, got %v", err)
		}
		if exitErr.ExitCode() != 1 {
			t.Fatalf("bootstrap with lock held: exit %d; want 1; stderr: %s", exitErr.ExitCode(), exitErr.Stderr)
		}
		if !strings.Contains(string(exitErr.Stderr), "another process holds the lock") {
			t.Fatalf("bootstrap with lock held: stderr %q does not name the lock", exitErr.Stderr)
		}

		for _, args := range [][]string{{"service", "start"}, {"service", "stop"}} {
			_, err = gr.Output(ctx, sagDevxtest, args...)
			if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
				t.Fatalf("%v with lock held: %v; want exit 1", args, err)
			}
			if !strings.Contains(string(exitErr.Stderr), "another process holds the lock") {
				t.Fatalf("%v with lock held: stderr %q does not name the lock", args, exitErr.Stderr)
			}
		}

		if runtime.GOOS == "linux" {
			t.Run("uid0_exits_one", func(t *testing.T) {
				cmd := exec.CommandContext(ctx, "sudo", "-n", sagDevxtest, "bootstrap", "--autostart", "no")
				out, err := cmd.CombinedOutput()
				var exitErr2 *exec.ExitError
				if !errors.As(err, &exitErr2) {
					t.Fatalf("sudo -n sag bootstrap: want exit error, got %v; output: %s", err, out)
				}
				if exitErr2.ExitCode() != 1 {
					t.Fatalf("sudo -n sag bootstrap: exit %d; want 1; output: %s", exitErr2.ExitCode(), out)
				}
				if !strings.Contains(string(out), "refused to run as UID 0") {
					t.Fatalf("sudo -n sag bootstrap: output %q does not say 'refused to run as UID 0'", out)
				}
			})
		}
	})
}
