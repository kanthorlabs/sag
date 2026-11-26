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

func testDXAC04BootstrapNoTerminal(t *testing.T, ctx context.Context, groupDir, binDir string) {
	t.Helper()
	home := os.Getenv("HOME")
	cfg := cfgDir(home)

	sagBin, err := dxh.TestBinary()
	if err != nil {
		t.Fatalf("DXAC04: test binary: %v", err)
	}

	env, err := dxh.ChildEnv(os.Environ(), groupDir)
	if err != nil {
		t.Fatalf("DXAC04: child env: %v", err)
	}
	env = append(env, "PATH="+dxh.TestPATH(binDir))

	testCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(testCtx, sagBin, "bootstrap")
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
			t.Fatalf("DXAC04: run sag-devxtest bootstrap: %v", runErr)
		}
	}

	if code != 3 {
		t.Fatalf("DXAC04: exit code = %d; want 3; stdout=%s stderr=%s", code, stdoutBuf.String(), stderrBuf.String())
	}

	combined := stdoutBuf.String() + stderrBuf.String()
	if !strings.Contains(combined, "--autostart") {
		t.Fatalf("DXAC04: output does not name --autostart; stdout=%s stderr=%s", stdoutBuf.String(), stderrBuf.String())
	}

	if _, statErr := os.Stat(cfg); statErr == nil {
		t.Fatalf("DXAC04: cfg dir %s exists after exit 3; want absent", cfg)
	}

	r := dxh.NewRunner()
	for _, name := range []string{"sag-master-key", "sag-tls-cert", "sag-tls-key"} {
		out, err := r.Output(ctx, "podman", "secret", "ls", "--filter", "name=^"+name+"$", "--format", "{{.Name}}")
		if err != nil {
			t.Fatalf("DXAC04: podman secret ls %s: %v", name, err)
		}
		if strings.TrimSpace(string(out)) != "" {
			t.Fatalf("DXAC04: secret %s exists after exit 3; want absent", name)
		}
	}
}
