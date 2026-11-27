//go:build devxe2e

package devx_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/kanthorlabs/sag/e2e/devx/dxh"
)

func init() {
	agentTests["DXAC43_AgentSetupRefusals"] = func(t *testing.T, ctx context.Context) {
		DXAC43_AgentSetupRefusals(t, ctx)
	}
}

func DXAC43_AgentSetupRefusals(t *testing.T, ctx context.Context) {
	t.Helper()
	dxh.EnsureBootstrapped(t)

	bin, err := dxh.TestBinary()
	if err != nil {
		t.Fatalf("DXAC43: test binary: %v", err)
	}
	podmanPath, err := exec.LookPath("podman")
	if err != nil {
		t.Fatalf("DXAC43: find podman: %v", err)
	}

	t.Run("claude_code_no_terminal_no_flags", func(t *testing.T) {
		dxac43NoTerminalNoFlags(t, ctx, bin, podmanPath, "claude-code", "--deny-rules")
	})

	t.Run("pi_no_terminal_no_flags", func(t *testing.T) {
		dxac43NoTerminalNoFlags(t, ctx, bin, podmanPath, "pi", "--shell-profile")
	})

	t.Run("existing_mcp_command_key", func(t *testing.T) {
		dxac43ExistingMCPCommandKey(t, ctx, bin, podmanPath)
	})

	t.Run("guard_without_marker", func(t *testing.T) {
		dxac43GuardWithoutMarker(t, ctx, bin, podmanPath)
	})

	t.Run("skill_without_marker", func(t *testing.T) {
		dxac43SkillWithoutMarker(t, ctx, bin, podmanPath)
	})

	t.Run("invalid_settings_json", func(t *testing.T) {
		dxac43InvalidSettingsJSON(t, ctx, bin, podmanPath)
	})

	t.Run("fish_shell_refused", func(t *testing.T) {
		dxac43FishShellRefused(t, ctx, bin, podmanPath)
	})
}

func dxac43NoTerminalNoFlags(t *testing.T, ctx context.Context, bin, podmanPath, harness, flagName string) {
	t.Helper()
	claudeDir := t.TempDir()
	piDir := t.TempDir()
	binDir := t.TempDir()
	if err := dxh.AddRealProgramLink(binDir, "podman", podmanPath); err != nil {
		t.Fatal(err)
	}

	env := dxac43BaseEnv(claudeDir, piDir, binDir)
	code, out := dxac43RunSetsid(ctx, bin, env, "agent", "setup", harness)
	if code != 3 {
		t.Fatalf("DXAC43 %s no-flags: expected exit 3; got %d\n%s", harness, code, out)
	}
	if !strings.Contains(out, flagName) {
		t.Errorf("DXAC43 %s no-flags: output does not name %s; got %q", harness, flagName, out)
	}
	dxac43AssertNoDirChanges(t, claudeDir, piDir)
}

func dxac43ExistingMCPCommandKey(t *testing.T, ctx context.Context, bin, podmanPath string) {
	t.Helper()
	claudeDir := t.TempDir()
	piDir := t.TempDir()
	binDir := t.TempDir()
	if err := dxh.AddRealProgramLink(binDir, "podman", podmanPath); err != nil {
		t.Fatal(err)
	}

	mcpWithCommand := map[string]interface{}{
		"mcpServers": map[string]interface{}{
			"sag": map[string]interface{}{
				"command": "sag",
				"args":    []string{"serve"},
			},
		},
	}
	mcpData, _ := json.Marshal(mcpWithCommand)
	mcpPath := filepath.Join(claudeDir, ".claude.json")
	if err := os.WriteFile(mcpPath, mcpData, 0o640); err != nil {
		t.Fatal(err)
	}

	env := dxac43BaseEnv(claudeDir, piDir, binDir)
	code, out := dxac43RunSetsid(ctx, bin, env, "agent", "setup", "claude-code", "--deny-rules", "yes")
	if code != 1 {
		t.Fatalf("DXAC43 mcp-command-key: expected exit 1; got %d\n%s", code, out)
	}
	dxac43AssertNoDirChanges(t, claudeDir, piDir)
}

func dxac43GuardWithoutMarker(t *testing.T, ctx context.Context, bin, podmanPath string) {
	t.Helper()
	piDir := t.TempDir()
	binDir := t.TempDir()
	if err := dxh.AddRealProgramLink(binDir, "podman", podmanPath); err != nil {
		t.Fatal(err)
	}

	extDir := filepath.Join(piDir, "extensions")
	if err := os.MkdirAll(extDir, 0o700); err != nil {
		t.Fatal(err)
	}
	guardPath := filepath.Join(extDir, "sag-guard.ts")
	if err := os.WriteFile(guardPath, []byte("// not managed by sag\nexport {};\n"), 0o640); err != nil {
		t.Fatal(err)
	}

	claudeDir := t.TempDir()
	env := dxac43BaseEnv(claudeDir, piDir, binDir)
	env = append(env, "SHELL=/bin/zsh")
	env = append(env, "ZDOTDIR="+t.TempDir())
	code, out := dxac43RunSetsid(ctx, bin, env, "agent", "setup", "pi",
		"--shell-profile", "yes", "--guard", "yes")
	if code != 1 {
		t.Fatalf("DXAC43 guard-no-marker: expected exit 1; got %d\n%s", code, out)
	}
	data, _ := os.ReadFile(guardPath)
	if !strings.HasPrefix(string(data), "// not managed by sag") {
		t.Error("DXAC43 guard-no-marker: guard file was overwritten despite no marker")
	}
}

func dxac43SkillWithoutMarker(t *testing.T, ctx context.Context, bin, podmanPath string) {
	t.Helper()
	claudeDir := t.TempDir()
	piDir := t.TempDir()
	binDir := t.TempDir()
	if err := dxh.AddRealProgramLink(binDir, "podman", podmanPath); err != nil {
		t.Fatal(err)
	}

	skillDir := filepath.Join(claudeDir, "skills", "sag")
	if err := os.MkdirAll(skillDir, 0o700); err != nil {
		t.Fatal(err)
	}
	skillPath := filepath.Join(skillDir, "SKILL.md")
	if err := os.WriteFile(skillPath, []byte("# not managed by sag\n"), 0o640); err != nil {
		t.Fatal(err)
	}

	env := dxac43BaseEnv(claudeDir, piDir, binDir)
	code, out := dxac43RunSetsid(ctx, bin, env, "agent", "setup", "claude-code", "--deny-rules", "yes")
	if code != 1 {
		t.Fatalf("DXAC43 skill-no-marker: expected exit 1; got %d\n%s", code, out)
	}
	data, _ := os.ReadFile(skillPath)
	if !strings.HasPrefix(string(data), "# not managed by sag") {
		t.Error("DXAC43 skill-no-marker: SKILL.md was overwritten despite no marker")
	}
}

func dxac43InvalidSettingsJSON(t *testing.T, ctx context.Context, bin, podmanPath string) {
	t.Helper()
	claudeDir := t.TempDir()
	piDir := t.TempDir()
	binDir := t.TempDir()
	if err := dxh.AddRealProgramLink(binDir, "podman", podmanPath); err != nil {
		t.Fatal(err)
	}

	settingsPath := filepath.Join(claudeDir, "settings.json")
	if err := os.WriteFile(settingsPath, []byte("{not valid json}"), 0o640); err != nil {
		t.Fatal(err)
	}

	env := dxac43BaseEnv(claudeDir, piDir, binDir)
	code, out := dxac43RunSetsid(ctx, bin, env, "agent", "setup", "claude-code", "--deny-rules", "yes")
	if code != 1 {
		t.Fatalf("DXAC43 invalid-json: expected exit 1; got %d\n%s", code, out)
	}
	data, _ := os.ReadFile(settingsPath)
	if string(data) != "{not valid json}" {
		t.Error("DXAC43 invalid-json: settings.json was modified despite parse failure")
	}
}

func dxac43FishShellRefused(t *testing.T, ctx context.Context, bin, podmanPath string) {
	t.Helper()
	piDir := t.TempDir()
	claudeDir := t.TempDir()
	binDir := t.TempDir()
	if err := dxh.AddRealProgramLink(binDir, "podman", podmanPath); err != nil {
		t.Fatal(err)
	}

	env := dxac43BaseEnv(claudeDir, piDir, binDir)
	env = append(env, "SHELL=/bin/fish")
	code, out := dxac43RunSetsid(ctx, bin, env, "agent", "setup", "pi",
		"--shell-profile", "yes", "--guard", "yes")
	if code != 1 {
		t.Fatalf("DXAC43 fish-shell: expected exit 1; got %d\n%s", code, out)
	}
	dxac43AssertNoDirChanges(t, claudeDir, piDir)
}

func dxac43BaseEnv(claudeDir, piDir, binDir string) []string {
	var env []string
	for _, e := range os.Environ() {
		key, _, _ := strings.Cut(e, "=")
		switch key {
		case "HOME", "TMPDIR", "USER", "LANG":
			env = append(env, e)
		default:
			if strings.HasPrefix(key, "SAG_TEST_") {
				env = append(env, e)
			}
		}
	}
	env = append(env, "CLAUDE_CONFIG_DIR="+claudeDir)
	env = append(env, "PI_CODING_AGENT_DIR="+piDir)
	env = append(env, "PATH="+dxh.TestPATH(binDir))
	return env
}

func dxac43RunSetsid(ctx context.Context, bin string, env []string, args ...string) (int, string) {
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = env
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	runErr := cmd.Run()
	code := 0
	if runErr != nil {
		if exitErr, ok := runErr.(*exec.ExitError); ok {
			code = exitErr.ExitCode()
		}
	}
	return code, out.String()
}

func dxac43AssertNoDirChanges(t *testing.T, dirs ...string) {
	t.Helper()
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		if len(entries) > 0 {
			names := make([]string, 0, len(entries))
			for _, e := range entries {
				names = append(names, e.Name())
			}
			t.Errorf("DXAC43: unexpected files in %s: %v", dir, names)
		}
	}
}
