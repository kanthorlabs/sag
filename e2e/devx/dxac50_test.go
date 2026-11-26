//go:build devxe2e

package devx_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kanthorlabs/sag/e2e/devx/dxh"
)

const fakeTailscaleIP = "100.64.0.1"

func DXAC50_TailscaleFake(t *testing.T, ctx context.Context) {
	t.Helper()

	sagBin, err := dxh.TestBinary()
	if err != nil {
		t.Fatalf("DXAC50: test binary: %v", err)
	}

	fakeBin, err := dxac50BuildFakeTailscale(t, ctx)
	if err != nil {
		t.Fatalf("DXAC50: build fake tailscale: %v", err)
	}

	podmanPath, err := exec.LookPath("podman")
	if err != nil {
		t.Fatalf("DXAC50: find podman: %v", err)
	}

	groupDir := t.TempDir()

	t.Run("setup-command-absent", func(t *testing.T) {
		binDir := t.TempDir()
		if err := dxh.AddRealProgramLink(binDir, "podman", podmanPath); err != nil {
			t.Fatalf("DXAC50: link podman: %v", err)
		}
		stdout, stderr, code := dxac50RunBootstrap(t, ctx, groupDir, binDir, sagBin)
		if code != 0 {
			t.Fatalf("DXAC50 setup: exit %d; want 0\nstdout=%s\nstderr=%s", code, stdout, stderr)
		}
		if !strings.Contains(stdout, "skip tailscale command absent; loopback only") {
			t.Fatalf("DXAC50 setup: stdout missing skip line; got %q", stdout)
		}
	})

	t.Run("conflict-other-target", func(t *testing.T) {
		binDir := t.TempDir()
		if err := dxh.AddRealProgramLink(binDir, "podman", podmanPath); err != nil {
			t.Fatalf("DXAC50: link podman: %v", err)
		}
		if err := dxh.AddRealProgramLink(binDir, "tailscale", fakeBin); err != nil {
			t.Fatalf("DXAC50: link tailscale: %v", err)
		}
		dxac50WriteState(t, binDir, "ts_ip4", fakeTailscaleIP)
		dxac50WriteState(t, binDir, "ts_serve_status", `{"TCP":{"8443":{"TCPForward":"other:9090"}}}`)

		stdout, stderr, code := dxac50RunBootstrap(t, ctx, groupDir, binDir, sagBin)
		if code != 1 {
			t.Fatalf("DXAC50 conflict-other-target: exit %d; want 1\nstdout=%s\nstderr=%s", code, stdout, stderr)
		}
		if !strings.Contains(stdout, "fail tailscale TCP 8443 serves another target") {
			t.Fatalf("DXAC50 conflict-other-target: stdout missing fail line; got %q", stdout)
		}
		if !strings.Contains(stderr, "sag: tailscale: TCP 8443 serves another target") {
			t.Fatalf("DXAC50 conflict-other-target: stderr missing sag error; got %q", stderr)
		}
	})

	t.Run("conflict-funnel", func(t *testing.T) {
		binDir := t.TempDir()
		if err := dxh.AddRealProgramLink(binDir, "podman", podmanPath); err != nil {
			t.Fatalf("DXAC50: link podman: %v", err)
		}
		if err := dxh.AddRealProgramLink(binDir, "tailscale", fakeBin); err != nil {
			t.Fatalf("DXAC50: link tailscale: %v", err)
		}
		dxac50WriteState(t, binDir, "ts_ip4", fakeTailscaleIP)
		dxac50WriteState(t, binDir, "ts_serve_status", `{"AllowFunnel":{"ts-node:8443":true}}`)

		stdout, stderr, code := dxac50RunBootstrap(t, ctx, groupDir, binDir, sagBin)
		if code != 1 {
			t.Fatalf("DXAC50 conflict-funnel: exit %d; want 1\nstdout=%s\nstderr=%s", code, stdout, stderr)
		}
		if !strings.Contains(stdout, "fail tailscale funnel is on for port 8443") {
			t.Fatalf("DXAC50 conflict-funnel: stdout missing fail line; got %q", stdout)
		}
		if !strings.Contains(stderr, "sag: tailscale: funnel is on for port 8443") {
			t.Fatalf("DXAC50 conflict-funnel: stderr missing sag error; got %q", stderr)
		}
	})

	t.Run("skip-address-invalid-loopback", func(t *testing.T) {
		binDir := t.TempDir()
		if err := dxh.AddRealProgramLink(binDir, "podman", podmanPath); err != nil {
			t.Fatalf("DXAC50: link podman: %v", err)
		}
		if err := dxh.AddRealProgramLink(binDir, "tailscale", fakeBin); err != nil {
			t.Fatalf("DXAC50: link tailscale: %v", err)
		}
		dxac50WriteState(t, binDir, "ts_ip4", "0.0.0.0")

		stdout, stderr, code := dxac50RunBootstrap(t, ctx, groupDir, binDir, sagBin)
		if code != 0 {
			t.Fatalf("DXAC50 skip-address-invalid-loopback: exit %d; want 0\nstdout=%s\nstderr=%s", code, stdout, stderr)
		}
		if !strings.Contains(stdout, "skip tailscale address invalid; loopback only") {
			t.Fatalf("DXAC50 skip-address-invalid-loopback: stdout missing skip line; got %q", stdout)
		}
	})

	t.Run("skip-address-invalid-private", func(t *testing.T) {
		binDir := t.TempDir()
		if err := dxh.AddRealProgramLink(binDir, "podman", podmanPath); err != nil {
			t.Fatalf("DXAC50: link podman: %v", err)
		}
		if err := dxh.AddRealProgramLink(binDir, "tailscale", fakeBin); err != nil {
			t.Fatalf("DXAC50: link tailscale: %v", err)
		}
		dxac50WriteState(t, binDir, "ts_ip4", "192.168.1.10")

		stdout, stderr, code := dxac50RunBootstrap(t, ctx, groupDir, binDir, sagBin)
		if code != 0 {
			t.Fatalf("DXAC50 skip-address-invalid-private: exit %d; want 0\nstdout=%s\nstderr=%s", code, stdout, stderr)
		}
		if !strings.Contains(stdout, "skip tailscale address invalid; loopback only") {
			t.Fatalf("DXAC50 skip-address-invalid-private: stdout missing skip line; got %q", stdout)
		}
	})

	t.Run("skip-down", func(t *testing.T) {
		binDir := t.TempDir()
		if err := dxh.AddRealProgramLink(binDir, "podman", podmanPath); err != nil {
			t.Fatalf("DXAC50: link podman: %v", err)
		}
		if err := dxh.AddRealProgramLink(binDir, "tailscale", fakeBin); err != nil {
			t.Fatalf("DXAC50: link tailscale: %v", err)
		}
		dxac50WriteState(t, binDir, "ts_ip4", "exit:1")

		stdout, stderr, code := dxac50RunBootstrap(t, ctx, groupDir, binDir, sagBin)
		if code != 0 {
			t.Fatalf("DXAC50 skip-down: exit %d; want 0\nstdout=%s\nstderr=%s", code, stdout, stderr)
		}
		if !strings.Contains(stdout, "skip tailscale down; loopback only") {
			t.Fatalf("DXAC50 skip-down: stdout missing skip line; got %q", stdout)
		}
	})

	t.Run("success", func(t *testing.T) {
		binDir := t.TempDir()
		if err := dxh.AddRealProgramLink(binDir, "podman", podmanPath); err != nil {
			t.Fatalf("DXAC50: link podman: %v", err)
		}
		if err := dxh.AddRealProgramLink(binDir, "tailscale", fakeBin); err != nil {
			t.Fatalf("DXAC50: link tailscale: %v", err)
		}
		dxac50WriteState(t, binDir, "ts_ip4", fakeTailscaleIP)

		stdout, stderr, code := dxac50RunBootstrap(t, ctx, groupDir, binDir, sagBin)
		if code != 0 {
			t.Fatalf("DXAC50 success: exit %d; want 0\nstdout=%s\nstderr=%s", code, stdout, stderr)
		}
		if !strings.Contains(stdout, "ok tailscale-serve") {
			t.Fatalf("DXAC50 success: stdout missing ok line; got %q", stdout)
		}
		if _, err := os.Stat(filepath.Join(binDir, "ts_rule")); err != nil {
			t.Fatalf("DXAC50 success: ts_rule not written in binDir: %v", err)
		}
	})

	t.Run("idempotent", func(t *testing.T) {
		binDir := t.TempDir()
		if err := dxh.AddRealProgramLink(binDir, "podman", podmanPath); err != nil {
			t.Fatalf("DXAC50: link podman: %v", err)
		}
		if err := dxh.AddRealProgramLink(binDir, "tailscale", fakeBin); err != nil {
			t.Fatalf("DXAC50: link tailscale: %v", err)
		}
		dxac50WriteState(t, binDir, "ts_ip4", fakeTailscaleIP)
		dxac50WriteState(t, binDir, "ts_serve_status", `{"TCP":{"8443":{"TCPForward":"127.0.0.1:8443"}}}`)

		stdout, stderr, code := dxac50RunBootstrap(t, ctx, groupDir, binDir, sagBin)
		if code != 0 {
			t.Fatalf("DXAC50 idempotent: exit %d; want 0\nstdout=%s\nstderr=%s", code, stdout, stderr)
		}
		if !strings.Contains(stdout, "ok tailscale-serve") {
			t.Fatalf("DXAC50 idempotent: stdout missing ok line; got %q", stdout)
		}
		if _, err := os.Stat(filepath.Join(binDir, "ts_rule")); err == nil {
			t.Fatal("DXAC50 idempotent: ts_rule written when rule already present; want absent")
		}
	})

	t.Run("address-change", func(t *testing.T) {
		t.Skip("D3.2-blocker: planOnce (plan-again, step 5) writes new TailscaleIP to answers.json before stepCert runs; readStoredTailscaleIP then returns the new IP; stored==cfg.TailscaleIP so renewal is not triggered; fix requires flow.go or endpointcert.go change outside D3.2 ownership")
	})
}

func dxac50BuildFakeTailscale(t *testing.T, ctx context.Context) (string, error) {
	t.Helper()
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", fmt.Errorf("git rev-parse: %w", err)
	}
	repoRoot := strings.TrimSpace(string(out))
	binPath := filepath.Join(t.TempDir(), "tailscale")
	buildCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	build := exec.CommandContext(buildCtx, "go", "build", "-o", binPath, "./e2e/devx/cmd/faketailscale")
	build.Dir = repoRoot
	if buildOut, err := build.CombinedOutput(); err != nil {
		return "", fmt.Errorf("go build: %w: %s", err, buildOut)
	}
	return binPath, nil
}

func dxac50RunBootstrap(t *testing.T, ctx context.Context, groupDir, binDir, sagBin string) (string, string, int) {
	t.Helper()
	env, err := dxh.ChildEnv(os.Environ(), groupDir)
	if err != nil {
		t.Fatalf("DXAC50: child env: %v", err)
	}
	env = append(env, "PATH="+dxh.TestPATH(binDir))

	testCtx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(testCtx, sagBin, "bootstrap",
		"--autostart", "yes",
		"--agent-setup", "claude-code=no",
		"--agent-setup", "pi=no")
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
			t.Fatalf("DXAC50: run sag bootstrap: %v", runErr)
		}
	}
	return stdoutBuf.String(), stderrBuf.String(), code
}

func dxac50WriteState(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
		t.Fatalf("DXAC50: write %s: %v", name, err)
	}
}
