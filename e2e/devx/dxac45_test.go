//go:build devxe2e

package devx_test

import (
	"bytes"
	"context"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/kanthorlabs/sag/e2e/devx/dxh"
)

func init() {
	agentTests["DXAC45_AgentIssue"] = func(t *testing.T, ctx context.Context) {
		DXAC45_AgentIssue(t, ctx)
	}
}

func DXAC45_AgentIssue(t *testing.T, ctx context.Context) {
	t.Helper()
	dxh.EnsureBootstrapped(t)

	t.Run("podman_exec_issues_jwt", func(t *testing.T) {
		testCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		r := dxh.NewRunner()
		out, err := r.Output(testCtx, "podman", "exec", "sag", "sag", "agent", "issue", "claude-code-dev")
		if err != nil {
			t.Fatalf("DXAC45-A: podman exec sag sag agent issue claude-code-dev: %v", err)
		}
		jwt := strings.TrimSpace(string(out))
		if !strings.HasPrefix(jwt, "eyJ") {
			t.Errorf("DXAC45-A: output %q: not a JWT", jwt)
		}
	})

	t.Run("no_terminal_exit3", func(t *testing.T) {
		testCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		sagBin, err := dxh.TestBinary()
		if err != nil {
			t.Fatalf("DXAC45-B: test binary: %v", err)
		}
		cmd := exec.CommandContext(testCtx, "podman", "exec", "sag", sagBin, "agent", "issue", "claude-code-dev")
		cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
		var buf bytes.Buffer
		cmd.Stdout = &buf
		cmd.Stderr = &buf
		runErr := cmd.Run()
		code := 0
		if runErr != nil {
			if exitErr, ok := runErr.(*exec.ExitError); ok {
				code = exitErr.ExitCode()
			} else {
				t.Fatalf("DXAC45-B: run: %v", runErr)
			}
		}
		combined := buf.String()
		if strings.Contains(combined, "eyJ") {
			t.Fatal("DXAC45-B: output contains JWT; want none when stdout is not a terminal")
		}
		if code == 0 {
			t.Fatalf("DXAC45-B: exit = 0; want non-zero (stdout is not a terminal, container exits 2 or we expect exit 2 for internal command on host)")
		}
	})
}
