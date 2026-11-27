//go:build devxe2e

package devx_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/kanthorlabs/sag/e2e/devx/dxh"
)

func init() {
	bootagentTests["DXAC46_BootstrapAgentSetup"] = DXAC46_BootstrapAgentSetup
}

func DXAC46_BootstrapAgentSetup(t *testing.T, ctx context.Context) {
	t.Helper()

	sagBin, err := dxh.TestBinary()
	if err != nil {
		t.Fatalf("DXAC46: test binary: %v", err)
	}

	binDir := t.TempDir()
	if err := dxac46BuildFakeHarness(binDir); err != nil {
		t.Fatalf("DXAC46: build fakeharness: %v", err)
	}

	t.Run("happy", func(t *testing.T) {
		env, err := dxh.ChildEnv(os.Environ(), t.TempDir())
		if err != nil {
			t.Fatalf("DXAC46/happy: child env: %v", err)
		}
		env = append(env, "PATH="+dxh.TestPATH(binDir))

		testCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
		defer cancel()

		cmd := exec.CommandContext(testCtx, sagBin, "bootstrap",
			"--autostart", "no",
			"--agent-setup", "claude-code=yes",
			"--agent-setup", "pi=no",
			"--deny-rules", "no")
		cmd.Env = env
		var stdoutBuf, stderrBuf strings.Builder
		cmd.Stdout = &stdoutBuf
		cmd.Stderr = &stderrBuf
		runErr := cmd.Run()
		code := 0
		if runErr != nil {
			if exitErr, ok := runErr.(*exec.ExitError); ok {
				code = exitErr.ExitCode()
			} else {
				t.Fatalf("DXAC46/happy: run: %v", runErr)
			}
		}
		if code != 0 {
			t.Fatalf("DXAC46/happy: exit code = %d; want 0; stdout=%s stderr=%s",
				code, stdoutBuf.String(), stderrBuf.String())
		}
		combined := stdoutBuf.String() + stderrBuf.String()
		if !strings.Contains(combined, "--agent claude-code") {
			t.Fatalf("DXAC46/happy: output missing --agent claude-code; got:\n%s", combined)
		}
		if strings.Contains(combined, "--agent pi") {
			t.Fatalf("DXAC46/happy: output has --agent pi; want absent; got:\n%s", combined)
		}
	})

	t.Run("noterm", func(t *testing.T) {
		env, err := dxh.ChildEnv(os.Environ(), t.TempDir())
		if err != nil {
			t.Fatalf("DXAC46/noterm: child env: %v", err)
		}
		env = append(env, "PATH="+dxh.TestPATH(binDir))

		testCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()

		cmd := exec.CommandContext(testCtx, sagBin, "bootstrap", "--autostart", "no")
		cmd.Env = env
		cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
		var stdoutBuf, stderrBuf strings.Builder
		cmd.Stdout = &stdoutBuf
		cmd.Stderr = &stderrBuf
		runErr := cmd.Run()
		code := 0
		if runErr != nil {
			if exitErr, ok := runErr.(*exec.ExitError); ok {
				code = exitErr.ExitCode()
			} else {
				t.Fatalf("DXAC46/noterm: run: %v", runErr)
			}
		}
		if code != 3 {
			t.Fatalf("DXAC46/noterm: exit code = %d; want 3; stdout=%s stderr=%s",
				code, stdoutBuf.String(), stderrBuf.String())
		}
		combined := stdoutBuf.String() + stderrBuf.String()
		if !strings.Contains(combined, "--agent-setup") {
			t.Fatalf("DXAC46/noterm: output does not name --agent-setup; got:\n%s", combined)
		}
	})

	t.Run("catrust", func(t *testing.T) {
		caFile := filepath.Join(t.TempDir(), "other-ca.pem")
		if err := os.WriteFile(caFile, []byte("not-a-cert"), 0o600); err != nil {
			t.Fatalf("DXAC46/catrust: write fake ca file: %v", err)
		}

		env, err := dxh.ChildEnv(os.Environ(), t.TempDir())
		if err != nil {
			t.Fatalf("DXAC46/catrust: child env: %v", err)
		}
		env = append(env, "PATH="+dxh.TestPATH(binDir))
		env = append(env, "NODE_EXTRA_CA_CERTS="+caFile)

		testCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
		defer cancel()

		cmd := exec.CommandContext(testCtx, sagBin, "bootstrap",
			"--autostart", "no",
			"--agent-setup", "claude-code=yes",
			"--agent-setup", "pi=no",
			"--deny-rules", "no")
		cmd.Env = env
		var stdoutBuf, stderrBuf strings.Builder
		cmd.Stdout = &stdoutBuf
		cmd.Stderr = &stderrBuf
		runErr := cmd.Run()
		code := 0
		if runErr != nil {
			if exitErr, ok := runErr.(*exec.ExitError); ok {
				code = exitErr.ExitCode()
			} else {
				t.Fatalf("DXAC46/catrust: run: %v", runErr)
			}
		}
		if code != 1 {
			t.Fatalf("DXAC46/catrust: exit code = %d; want 1; stdout=%s stderr=%s",
				code, stdoutBuf.String(), stderrBuf.String())
		}
		combined := stdoutBuf.String() + stderrBuf.String()
		if !strings.Contains(combined, "NODE_EXTRA_CA_CERTS") {
			t.Fatalf("DXAC46/catrust: output missing NODE_EXTRA_CA_CERTS bundle hint; got:\n%s", combined)
		}

		home := os.Getenv("HOME")
		caPath := filepath.Join(dxh.CfgDir(home), "ca.pem")
		if err := healthzOK(ctx, caPath); err != nil {
			t.Fatalf("DXAC46/catrust: /healthz after ca-trust exit: %v", err)
		}
	})
}

func dxac46BuildFakeHarness(binDir string) error {
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return fmt.Errorf("git rev-parse: %w", err)
	}
	root := strings.TrimSpace(string(out))
	fakeBin := filepath.Join(binDir, "fakeharness")
	cmd := exec.Command("go", "build", "-o", fakeBin, "./e2e/devx/cmd/fakeharness/")
	cmd.Dir = root
	cmd.Env = os.Environ()
	if data, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("go build fakeharness: %w\n%s", err, data)
	}
	for _, name := range []string{"claude", "pi"} {
		link := filepath.Join(binDir, name)
		_ = os.Remove(link)
		if err := os.Symlink(fakeBin, link); err != nil {
			return fmt.Errorf("symlink %s: %w", name, err)
		}
	}
	return nil
}
