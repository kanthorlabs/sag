//go:build devxe2e

package devx_test

import (
	"bytes"
	"context"
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
	userHarnessTests["DXAC47_RealClaudeCode"] = func(t *testing.T, ctx context.Context) {
		DXAC47_RealClaudeCode(t, ctx)
	}
}

func DXAC47_RealClaudeCode(t *testing.T, ctx context.Context) {
	t.Helper()
	dxh.EnsureBootstrapped(t)

	claudePath, err := exec.LookPath("claude")
	if err != nil {
		t.Fatalf("DXAC47: find claude: %v", err)
	}

	r := dxh.NewRunner()
	if err := dxh.VersionGate(ctx, r, claudePath, dxh.ClaudeCodeVersion); err != nil {
		t.Fatalf("DXAC47: %v", err)
	}

	sagBin, err := dxh.TestBinary()
	if err != nil {
		t.Fatalf("DXAC47: test binary: %v", err)
	}

	home := os.Getenv("HOME")
	cfgDir := dxh.CfgDir(home)
	caPEM := filepath.Join(cfgDir, "ca.pem")

	setupEnv := dxac47Env(home, "NODE_EXTRA_CA_CERTS="+caPEM)

	setupCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(setupCtx, sagBin, "agent", "setup", "claude-code", "--deny-rules", "yes")
	cmd.Env = setupEnv
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	var setupOut bytes.Buffer
	cmd.Stdout = &setupOut
	cmd.Stderr = &setupOut
	if err := cmd.Run(); err != nil {
		t.Fatalf("DXAC47: agent setup: %v\n%s", err, dxh.FilterNoJWT(setupOut.String()))
	}

	claudeEnv := dxac47Env(home)

	t.Run("mcp_list_sag_connected", func(t *testing.T) {
		mcpCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
		defer cancel()
		mcpCmd := exec.CommandContext(mcpCtx, claudePath, "mcp", "list")
		mcpCmd.Env = claudeEnv
		out, err := mcpCmd.Output()
		if err != nil {
			t.Fatalf("DXAC47-A: claude mcp list: %v", err)
		}
		text := dxh.FilterNoJWT(string(out))
		if !strings.Contains(strings.ToLower(text), "sag") {
			t.Errorf("DXAC47-A: claude mcp list output does not mention sag:\n%s", text)
		}
	})

	t.Run("list_resources", func(t *testing.T) {
		listCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
		defer cancel()
		listCmd := exec.CommandContext(listCtx, claudePath, "-p", "--output-format", "json",
			"List my available SAG credentials by calling list_resources")
		listCmd.Env = claudeEnv
		out, err := listCmd.Output()
		if err != nil {
			t.Fatalf("DXAC47-B: claude -p list resources: %v\n%s", err, dxh.FilterNoJWT(string(out)))
		}
		if strings.TrimSpace(dxh.FilterNoJWT(string(out))) == "" {
			t.Errorf("DXAC47-B: claude -p output empty")
		}
	})

	t.Run("deny_rule_permission_denial", func(t *testing.T) {
		denyCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
		defer cancel()
		denyCmd := exec.CommandContext(denyCtx, claudePath,
			"-p",
			"--output-format", "json",
			"--allowedTools", "Bash(sag:*)",
			"Run this bash command: sag credential delete x")
		denyCmd.Env = claudeEnv
		out, _ := denyCmd.Output()
		filtered := dxh.FilterNoJWT(string(out))
		if !strings.Contains(filtered, "permission_denials") {
			t.Errorf("DXAC47-C: output has no permission_denials:\n%s", filtered)
		}
	})

	t.Run("resource_x_absent", func(t *testing.T) {
		checkCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		checkCmd := exec.CommandContext(checkCtx, sagBin, "credential", "list")
		checkCmd.Env = setupEnv
		out, err := checkCmd.Output()
		if err != nil {
			t.Fatalf("DXAC47-D: credential list: %v", err)
		}
		for _, line := range strings.Split(string(out), "\n") {
			if strings.TrimSpace(line) == "x" {
				t.Errorf("DXAC47-D: resource x exists; want absent")
			}
		}
	})
}

func dxac47Env(home string, extra ...string) []string {
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
