//go:build devxe2e

package devx_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/kanthorlabs/sag/e2e/devx/dxh"
)

func testDXAC36_HostGuards(t *testing.T, ctx context.Context, groupDir, binDir string) {
	t.Run("DXAC36_HostGuards", func(t *testing.T) {
		devDir, err := dxh.SharedDevDir()
		if err != nil {
			t.Fatalf("SharedDevDir: %v", err)
		}
		repoRoot := filepath.Dir(devDir)
		sagDevxtest := filepath.Join(repoRoot, "bin", "sag-devxtest")

		if err := dxh.AddRealProgramLink(binDir, "sag-devxtest", sagDevxtest); err != nil {
			t.Fatalf("AddRealProgramLink: %v", err)
		}

		gr := dxh.NewGroupRunner(groupDir, binDir)

		internalCmds := [][]string{
			{"sag-devxtest", "serve"},
			{"sag-devxtest", "credential", "check", "dummy"},
			{"sag-devxtest", "audit", "read"},
			{"sag-devxtest", "agent", "issue", "a"},
		}
		for _, args := range internalCmds {
			_, err := gr.Output(ctx, args[0], args[1:]...)
			var exitErr *exec.ExitError
			if !errors.As(err, &exitErr) {
				t.Errorf("%v: want exit error, got %v", args, err)
				continue
			}
			if exitErr.ExitCode() != 2 {
				t.Errorf("%v: exit %d; want 2", args, exitErr.ExitCode())
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

		_, err = gr.Output(ctx, "sag-devxtest", "service", "start")
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			t.Fatalf("service start with lock held: want exit error, got %v", err)
		}
		if exitErr.ExitCode() != 1 {
			t.Fatalf("service start with lock held: exit %d; want 1", exitErr.ExitCode())
		}
	})
}
