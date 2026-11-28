//go:build devxe2e

package devx_test

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/kanthorlabs/sag/e2e/devx/dxh"
)

func init() {
	userHarnessTests["DXAC48_RealPi"] = func(t *testing.T, ctx context.Context) {
		DXAC48_RealPi(t, ctx)
	}
}

func DXAC48_RealPi(t *testing.T, ctx context.Context) {
	t.Helper()
	dxh.EnsureBootstrapped(t)

	piPath, err := exec.LookPath("pi")
	if err != nil {
		t.Fatalf("DXAC48: find pi: %v", err)
	}

	r := dxh.NewRunner()
	if err := dxh.VersionGate(ctx, r, piPath, dxh.PiVersion); err != nil {
		t.Fatalf("DXAC48: %v", err)
	}

	sagBin, err := dxh.TestBinary()
	if err != nil {
		t.Fatalf("DXAC48: test binary: %v", err)
	}

	home := os.Getenv("HOME")
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/zsh"
	}

	setupEnv := dxac48Env(home)

	setupCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(setupCtx, sagBin, "agent", "setup", "pi",
		"--shell-profile", "yes", "--guard", "yes")
	cmd.Env = append(setupEnv, "SHELL="+shell)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	var setupOut bytes.Buffer
	cmd.Stdout = &setupOut
	cmd.Stderr = &setupOut
	if err := cmd.Run(); err != nil {
		t.Fatalf("DXAC48: agent setup pi: %v\n%s", err, dxh.FilterNoJWT(setupOut.String()))
	}

	t.Run("mcp_list_sag_connected", func(t *testing.T) {
		mcpCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
		defer cancel()
		mcpCmd := exec.CommandContext(mcpCtx, shell, "-i", "-c", piPath+" mcp list --json")
		mcpCmd.Env = dxac48LoginEnv(home, shell)
		out, err := mcpCmd.Output()
		if err != nil {
			t.Fatalf("DXAC48-A: pi mcp list --json: %v", err)
		}
		text := dxh.FilterNoJWT(string(out))
		if !strings.Contains(strings.ToLower(text), "sag") {
			t.Errorf("DXAC48-A: pi mcp list --json output does not mention sag:\n%s", text)
		}
		if !strings.Contains(text, "connected") {
			t.Errorf("DXAC48-A: pi mcp list --json output does not show connected:\n%s", text)
		}
	})

	t.Run("guard_blocks_podman_secret_rm", func(t *testing.T) {
		guardCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
		defer cancel()
		guardCmd := exec.CommandContext(guardCtx, piPath,
			"--mode", "json",
			"Run this bash command: podman secret rm x")
		guardCmd.Env = dxac48PiEnv(home, dxh.CfgDir(home))
		out, _ := guardCmd.Output()
		filtered := dxh.FilterNoJWT(string(out))
		if !strings.Contains(filtered, "sag-guard:") {
			t.Errorf("DXAC48-B: pi --mode json output has no sag-guard: reason:\n%s", filtered)
		}
	})
}

func dxac48Env(home string, extra ...string) []string {
	env := []string{"HOME=" + home}
	for _, e := range os.Environ() {
		key, _, _ := strings.Cut(e, "=")
		switch key {
		case "HOME", "CLAUDE_CONFIG_DIR", "PI_CODING_AGENT_DIR", "NODE_EXTRA_CA_CERTS":
			continue
		}
		env = append(env, e)
	}
	env = append(env, extra...)
	return env
}

func dxac48LoginEnv(home, shell string) []string {
	env := []string{
		"HOME=" + home,
		"SHELL=" + shell,
	}
	for _, e := range os.Environ() {
		key, _, _ := strings.Cut(e, "=")
		switch key {
		case "HOME", "SHELL", "CLAUDE_CONFIG_DIR", "PI_CODING_AGENT_DIR", "NODE_EXTRA_CA_CERTS":
			continue
		}
		env = append(env, e)
	}
	return env
}

func dxac48PiEnv(home, cfgDir string) []string {
	env := []string{
		"HOME=" + home,
		"NODE_EXTRA_CA_CERTS=" + cfgDir + "/ca.pem",
	}
	for _, e := range os.Environ() {
		key, _, _ := strings.Cut(e, "=")
		switch key {
		case "HOME", "NODE_EXTRA_CA_CERTS", "CLAUDE_CONFIG_DIR", "PI_CODING_AGENT_DIR":
			continue
		}
		env = append(env, e)
	}
	return env
}
