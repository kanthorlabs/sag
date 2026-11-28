//go:build devxe2e

package devx_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/kanthorlabs/sag/e2e/devx/dxh"
	"github.com/kanthorlabs/sag/internal/agentsetup"
)

func init() {
	userHarnessTests["DXAC47_RealClaudeCode"] = func(t *testing.T, ctx context.Context) {
		DXAC47_RealClaudeCode(t, ctx)
	}
}

func DXAC47_RealClaudeCode(t *testing.T, ctx context.Context) {
	t.Helper()
	dxh.EnsureBootstrapped(t)
	dxh.EnsureCanary(t)

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
		if err := dxh.ParseClaudeMCPListSAG(dxh.FilterNoJWT(string(out))); err != nil {
			t.Errorf("DXAC47-A: %v", err)
		}
	})

	t.Run("list_resources", func(t *testing.T) {
		const tempName = "testcc47"

		createCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
		defer cancel()
		createCmd := exec.CommandContext(createCtx, sagBin,
			"credential", "create", tempName,
			"--kind", "postgres",
			"--agent", "claude-code",
			"--tls", "disable",
			"--stdin",
			"--accept-failed-check",
		)
		createCmd.Env = setupEnv
		createCmd.Stdin = strings.NewReader(canaryHostDSN(t, ctx) + "\n")
		var createOut bytes.Buffer
		createCmd.Stdout = &createOut
		createCmd.Stderr = &createOut
		if err := createCmd.Run(); err != nil {
			t.Fatalf("DXAC47-B: credential create: %v\n%s", err, createOut.String())
		}
		defer func() {
			if code, tty := deleteViaStartTTY(t, context.Background(), tempName); code != 0 {
				t.Errorf("DXAC47-B: credential delete %s: exit %d\n%s", tempName, code, dxh.FilterNoJWT(tty.RawOutput()))
			}
		}()
		if err := healthzOK(ctx, caPEM); err != nil {
			t.Fatalf("DXAC47-B: /healthz after credential create: %v", err)
		}
		if err := dxh.WaitLaunchAgentIdle(ctx, r, fmt.Sprint(os.Getuid())); err != nil {
			t.Fatalf("DXAC47-B: wait idle after credential create: %v", err)
		}
		if names, found := dxac45WaitForResource(t, ctx, cfgDir, dxac47UserJWT(t, home), tempName); !found {
			t.Fatalf("DXAC47-B: list_resources over /mcp does not contain %q after 60 s; got %v", tempName, names)
		}

		listCtx, cancel2 := context.WithTimeout(ctx, 60*time.Second)
		defer cancel2()
		listCmd := exec.CommandContext(listCtx, claudePath,
			"-p", "--output-format", "json",
			"List my available SAG credentials by calling list_resources")
		listCmd.Env = claudeEnv
		out, err := listCmd.Output()
		if err != nil {
			t.Fatalf("DXAC47-B: claude -p list resources: %v\n%s", err, dxh.FilterNoJWT(string(out)))
		}
		if err := dxh.ParseClaudeListResources(dxh.FilterNoJWT(string(out)), tempName); err != nil {
			t.Errorf("DXAC47-B: %v\n%s", err, dxh.FilterNoJWT(string(out)))
		}
	})

	t.Run("deny_rule_permission_denial", func(t *testing.T) {
		denyCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
		defer cancel()
		denyCmd := exec.CommandContext(denyCtx, claudePath,
			"-p", "Run this bash command: sag credential delete x",
			"--output-format", "json",
			"--allowedTools", "Bash(sag:*)")
		denyCmd.Env = claudeEnv
		var denyErr bytes.Buffer
		denyCmd.Stderr = &denyErr
		out, _ := denyCmd.Output()
		filtered := dxh.FilterNoJWT(string(out))
		if err := dxh.ParseClaudePermissionDenials(filtered, "sag credential delete x"); err != nil {
			t.Errorf("DXAC47-C: %v\nstdout: %s\nstderr: %s", err, filtered, dxh.FilterNoJWT(denyErr.String()))
		}
	})

	t.Run("resource_x_absent", func(t *testing.T) {
		checkCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		checkCmd := exec.CommandContext(checkCtx, sagBin, "credential", "list", "--json")
		checkCmd.Env = setupEnv
		out, err := checkCmd.Output()
		if err != nil {
			t.Fatalf("DXAC47-D: credential list --json: %v", err)
		}
		if err := dxh.ParseCredentialListNoName(string(out), "x"); err != nil {
			t.Errorf("DXAC47-D: %v\n%s", err, string(out))
		}
	})
}

func dxac47Env(home string, extra ...string) []string {
	return append(dxh.UserChildEnv(os.Environ(), userRealBinDir), extra...)
}

func dxac47UserJWT(t *testing.T, home string) string {
	t.Helper()
	data, err := os.ReadFile(agentsetup.ClaudeCodePaths("", home).MCPFile)
	if err != nil {
		t.Fatalf("DXAC47: read .claude.json: %v", err)
	}
	var top struct {
		MCPServers map[string]struct {
			Headers map[string]string `json:"headers"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal(data, &top); err != nil {
		t.Fatalf("DXAC47: parse .claude.json: %v", err)
	}
	token, ok := strings.CutPrefix(top.MCPServers["sag"].Headers["Authorization"], "Bearer ")
	if !ok || token == "" {
		t.Fatal("DXAC47: mcpServers.sag has no Bearer token")
	}
	return token
}
