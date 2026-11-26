//go:build devxe2e

package devx_test

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/kanthorlabs/sag/e2e/devx/dxh"
)

func DXAC06_InstallApproval(t *testing.T, ctx context.Context) {
	t.Helper()

	groupDir := t.TempDir()
	binDir := t.TempDir()

	sagBin, err := dxh.TestBinary()
	if err != nil {
		t.Fatalf("DXAC06: test binary: %v", err)
	}

	testPins := "testpkg 0.1.0 darwin/arm64 release https://example.com/fake.pkg abc123placeholder\n"
	t.Setenv("SAG_TEST_ABSENT", "1")
	t.Setenv("SAG_TEST_PINS", testPins)

	childEnv, err := dxh.ChildEnv(os.Environ(), groupDir)
	if err != nil {
		t.Fatalf("DXAC06: child env: %v", err)
	}
	childEnv = append(childEnv, "PATH="+dxh.TestPATH(binDir))

	runNoTerminal := func(extraArgs ...string) (stdout, stderr string, code int) {
		testCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()

		args := []string{"bootstrap", "--autostart", "no", "--agent-setup", "claude-code=no", "--agent-setup", "pi=no"}
		args = append(args, extraArgs...)

		cmd := exec.CommandContext(testCtx, sagBin, args...)
		cmd.Env = childEnv
		cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

		var stdoutBuf, stderrBuf strings.Builder
		cmd.Stdout = &stdoutBuf
		cmd.Stderr = &stderrBuf

		runErr := cmd.Run()
		if runErr != nil {
			if exitErr, ok := runErr.(*exec.ExitError); ok {
				code = exitErr.ExitCode()
			} else {
				t.Fatalf("DXAC06: run: %v", runErr)
			}
		}
		return stdoutBuf.String(), stderrBuf.String(), code
	}

	home := os.Getenv("HOME")
	cfg := cfgDir(home)

	stdout, stderr, code := runNoTerminal()
	if code != 3 {
		t.Fatalf("DXAC06 no-approval: exit code = %d; want 3\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}
	wantFailLine := "fail testpkg absent; run on a terminal or add --auto-approve testpkg@0.1.0"
	if !strings.Contains(stdout, wantFailLine) {
		t.Fatalf("DXAC06 no-approval: stdout = %q; want line %q", stdout, wantFailLine)
	}
	wantStderr := "sag: install of testpkg requires approval; add --auto-approve testpkg@0.1.0"
	if !strings.Contains(stderr, wantStderr) {
		t.Fatalf("DXAC06 no-approval: stderr = %q; want %q", stderr, wantStderr)
	}
	if _, err := os.Stat(cfg); err == nil {
		t.Fatalf("DXAC06 no-approval: cfg dir exists after exit 3: %s", cfg)
	}

	stdout, stderr, code = runNoTerminal("--auto-approve", "testpkg@0.0.1")
	if code != 3 {
		t.Fatalf("DXAC06 wrong-version: exit code = %d; want 3\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}
	wantWarnLine := "warn testpkg pre-approval testpkg@0.0.1 does not match pin 0.1.0; ignored"
	if !strings.Contains(stdout, wantWarnLine) {
		t.Fatalf("DXAC06 wrong-version: stdout = %q; want line %q", stdout, wantWarnLine)
	}
	if !strings.Contains(stdout, wantFailLine) {
		t.Fatalf("DXAC06 wrong-version: stdout = %q; want fail line %q after warn", stdout, wantFailLine)
	}
	if !strings.Contains(stderr, wantStderr) {
		t.Fatalf("DXAC06 wrong-version: stderr = %q; want %q", stderr, wantStderr)
	}
	if _, err := os.Stat(cfg); err == nil {
		t.Fatalf("DXAC06 wrong-version: cfg dir exists after exit 3: %s", cfg)
	}
}
