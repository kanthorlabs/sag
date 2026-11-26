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

const (
	fakeTailscaleIP    = "100.64.0.1"
	fakeTailscaleIPAlt = "100.64.1.2"
)

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
	binDir := t.TempDir()

	if err := dxh.AddRealProgramLink(binDir, "podman", podmanPath); err != nil {
		t.Fatalf("DXAC50: link podman: %v", err)
	}

	installFake := func(t *testing.T) {
		t.Helper()
		data, err := os.ReadFile(fakeBin)
		if err != nil {
			t.Fatalf("DXAC50: read fake binary: %v", err)
		}
		if err := os.WriteFile(filepath.Join(binDir, "tailscale"), data, 0o755); err != nil {
			t.Fatalf("DXAC50: install fake binary: %v", err)
		}
	}

	removeFake := func() {
		_ = os.Remove(filepath.Join(binDir, "tailscale"))
	}

	cleanState := func() {
		for _, f := range []string{"ts_ip4", "ts_serve_status", "ts_rule", "ts_args"} {
			_ = os.Remove(filepath.Join(binDir, f))
		}
	}

	writeState := func(t *testing.T, name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(binDir, name), []byte(content), 0o600); err != nil {
			t.Fatalf("DXAC50: write %s: %v", name, err)
		}
	}

	readState := func(name string) string {
		data, _ := os.ReadFile(filepath.Join(binDir, name))
		return string(data)
	}

	countLine := func(text, want string) int {
		count := 0
		for _, line := range strings.Split(text, "\n") {
			if line == want {
				count++
			}
		}
		return count
	}

	runBS := func(t *testing.T) (stdout, stderr string, code int) {
		t.Helper()
		return dxac50RunBootstrap(t, ctx, groupDir, binDir, sagBin)
	}

	t.Run("success", func(t *testing.T) {
		cleanState()
		installFake(t)
		writeState(t, "ts_ip4", fakeTailscaleIP)

		stdout, stderr, code := runBS(t)
		if code != 0 {
			t.Fatalf("DXAC50 success: exit %d; want 0\nstdout=%s\nstderr=%s", code, stdout, stderr)
		}
		if n := countLine(stdout, "install tailscale-serve"); n != 1 {
			t.Fatalf("DXAC50 success: install tailscale-serve appears %d times; want 1\nstdout=%q", n, stdout)
		}
		if _, err := os.Stat(filepath.Join(binDir, "ts_rule")); err != nil {
			t.Fatal("DXAC50 success: ts_rule not written")
		}
		args := readState("ts_args")
		if n := countLine(args, "serve --bg --tcp 8443 tcp://127.0.0.1:8443"); n != 1 {
			t.Fatalf("DXAC50 success: serve --bg appears %d times in ts_args; want 1\n%s", n, args)
		}
		if strings.Contains(args, "funnel") {
			t.Fatalf("DXAC50 success: funnel found in ts_args\n%s", args)
		}
	})

	t.Run("idempotent", func(t *testing.T) {
		cleanState()
		installFake(t)
		writeState(t, "ts_ip4", fakeTailscaleIP)
		writeState(t, "ts_serve_status", `{"TCP":{"8443":{"TCPForward":"127.0.0.1:8443"}}}`)

		stdout, stderr, code := runBS(t)
		if code != 0 {
			t.Fatalf("DXAC50 idempotent: exit %d; want 0\nstdout=%s\nstderr=%s", code, stdout, stderr)
		}
		if n := countLine(stdout, "ok tailscale-serve"); n != 1 {
			t.Fatalf("DXAC50 idempotent: ok tailscale-serve appears %d times; want 1\nstdout=%q", n, stdout)
		}
		if _, err := os.Stat(filepath.Join(binDir, "ts_rule")); err == nil {
			t.Fatal("DXAC50 idempotent: ts_rule written when rule already present; want absent")
		}
		args := readState("ts_args")
		if n := countLine(args, "serve --bg --tcp 8443 tcp://127.0.0.1:8443"); n != 0 {
			t.Fatalf("DXAC50 idempotent: serve --bg appears %d times in ts_args; want 0\n%s", n, args)
		}
		if strings.Contains(args, "funnel") {
			t.Fatalf("DXAC50 idempotent: funnel found in ts_args\n%s", args)
		}
	})

	t.Run("skip-down", func(t *testing.T) {
		cleanState()
		installFake(t)
		writeState(t, "ts_ip4", "exit:1")

		stdout, stderr, code := runBS(t)
		if code != 0 {
			t.Fatalf("DXAC50 skip-down: exit %d; want 0\nstdout=%s\nstderr=%s", code, stdout, stderr)
		}
		if n := countLine(stdout, "skip tailscale down; loopback only"); n != 1 {
			t.Fatalf("DXAC50 skip-down: skip line appears %d times; want 1\nstdout=%q", n, stdout)
		}
		args := readState("ts_args")
		if strings.Contains(args, "serve --bg") {
			t.Fatalf("DXAC50 skip-down: serve --bg in ts_args; want none\n%s", args)
		}
	})

	t.Run("skip-address-invalid-loopback", func(t *testing.T) {
		cleanState()
		installFake(t)
		writeState(t, "ts_ip4", "0.0.0.0")

		stdout, stderr, code := runBS(t)
		if code != 0 {
			t.Fatalf("DXAC50 skip-address-invalid-loopback: exit %d; want 0\nstdout=%s\nstderr=%s", code, stdout, stderr)
		}
		if n := countLine(stdout, "skip tailscale address invalid; loopback only"); n != 1 {
			t.Fatalf("DXAC50 skip-address-invalid-loopback: skip line appears %d times; want 1\nstdout=%q", n, stdout)
		}
	})

	t.Run("skip-address-invalid-private", func(t *testing.T) {
		cleanState()
		installFake(t)
		writeState(t, "ts_ip4", "192.168.1.10")

		stdout, stderr, code := runBS(t)
		if code != 0 {
			t.Fatalf("DXAC50 skip-address-invalid-private: exit %d; want 0\nstdout=%s\nstderr=%s", code, stdout, stderr)
		}
		if n := countLine(stdout, "skip tailscale address invalid; loopback only"); n != 1 {
			t.Fatalf("DXAC50 skip-address-invalid-private: skip line appears %d times; want 1\nstdout=%q", n, stdout)
		}
	})

	t.Run("skip-command-absent", func(t *testing.T) {
		cleanState()
		removeFake()

		stdout, stderr, code := runBS(t)
		if code != 0 {
			t.Fatalf("DXAC50 skip-command-absent: exit %d; want 0\nstdout=%s\nstderr=%s", code, stdout, stderr)
		}
		if n := countLine(stdout, "skip tailscale command absent; loopback only"); n != 1 {
			t.Fatalf("DXAC50 skip-command-absent: skip line appears %d times; want 1\nstdout=%q", n, stdout)
		}
	})

	t.Run("conflict-other-target", func(t *testing.T) {
		cleanState()
		installFake(t)
		writeState(t, "ts_ip4", fakeTailscaleIP)
		writeState(t, "ts_serve_status", `{"TCP":{"8443":{"TCPForward":"other:9090"}}}`)

		stdout, stderr, code := runBS(t)
		if code != 1 {
			t.Fatalf("DXAC50 conflict-other-target: exit %d; want 1\nstdout=%s\nstderr=%s", code, stdout, stderr)
		}
		if n := countLine(stdout, "fail tailscale TCP 8443 serves another target"); n != 1 {
			t.Fatalf("DXAC50 conflict-other-target: fail line appears %d times; want 1\nstdout=%q", n, stdout)
		}
		if !strings.Contains(stderr, "sag: tailscale: TCP 8443 serves another target") {
			t.Fatalf("DXAC50 conflict-other-target: stderr missing sag error; got %q", stderr)
		}
		args := readState("ts_args")
		if strings.Contains(args, "serve --bg") {
			t.Fatalf("DXAC50 conflict-other-target: serve --bg in ts_args; want none\n%s", args)
		}
		if strings.Contains(args, "funnel") {
			t.Fatalf("DXAC50 conflict-other-target: funnel in ts_args\n%s", args)
		}
	})

	t.Run("conflict-funnel", func(t *testing.T) {
		cleanState()
		installFake(t)
		writeState(t, "ts_ip4", fakeTailscaleIP)
		writeState(t, "ts_serve_status", `{"AllowFunnel":{"ts-node:8443":true}}`)

		stdout, stderr, code := runBS(t)
		if code != 1 {
			t.Fatalf("DXAC50 conflict-funnel: exit %d; want 1\nstdout=%s\nstderr=%s", code, stdout, stderr)
		}
		if n := countLine(stdout, "fail tailscale funnel is on for port 8443"); n != 1 {
			t.Fatalf("DXAC50 conflict-funnel: fail line appears %d times; want 1\nstdout=%q", n, stdout)
		}
		if !strings.Contains(stderr, "sag: tailscale: funnel is on for port 8443") {
			t.Fatalf("DXAC50 conflict-funnel: stderr missing sag error; got %q", stderr)
		}
		args := readState("ts_args")
		if strings.Contains(args, "funnel") {
			t.Fatalf("DXAC50 conflict-funnel: funnel in ts_args\n%s", args)
		}
	})

	t.Run("address-change", func(t *testing.T) {
		cleanState()
		installFake(t)
		writeState(t, "ts_ip4", fakeTailscaleIP)
		stdout1, stderr1, code1 := runBS(t)
		if code1 != 0 {
			t.Fatalf("DXAC50 address-change setup: exit %d; want 0\nstdout=%s\nstderr=%s", code1, stdout1, stderr1)
		}

		cleanState()
		installFake(t)
		writeState(t, "ts_ip4", fakeTailscaleIPAlt)
		stdout2, stderr2, code2 := runBS(t)
		if code2 != 0 {
			t.Fatalf("DXAC50 address-change run: exit %d; want 0\nstdout=%s\nstderr=%s", code2, stdout2, stderr2)
		}
		if !strings.Contains(stdout2, "install endpoint-cert") {
			t.Fatalf("DXAC50 address-change: cert not renewed when Tailscale IP changed\nstdout=%q\nD3.2-blocker: planOnce (plan-again, step 5) writes new TailscaleIP to answers.json before stepCert runs; readStoredTailscaleIP then returns the new IP; stored==cfg.TailscaleIP so renewal is not triggered; fix requires flow.go or endpointcert.go change outside D3.2 ownership", stdout2)
		}
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
