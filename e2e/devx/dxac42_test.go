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

	"github.com/kanthorlabs/sag/e2e/devx/dxh"
)

func init() {
	agentTests["DXAC42_CATrustConflict"] = func(t *testing.T, ctx context.Context) {
		DXAC42_CATrustConflict(t, ctx)
	}
}

func DXAC42_CATrustConflict(t *testing.T, ctx context.Context) {
	t.Helper()
	dxh.EnsureBootstrapped(t)

	bin, err := dxh.TestBinary()
	if err != nil {
		t.Fatalf("DXAC42: test binary: %v", err)
	}
	podmanPath, err := exec.LookPath("podman")
	if err != nil {
		t.Fatalf("DXAC42: find podman: %v", err)
	}

	fakeBinDir := dxh.ActiveAgentBinDir
	if fakeBinDir == "" {
		t.Skip("DXAC42: ActiveAgentBinDir not set; depends on D6.2 fakeharness")
	}

	t.Run("pi_env_conflict", func(t *testing.T) {
		dxac42PiEnvConflict(t, ctx, bin, podmanPath, fakeBinDir)
	})

	t.Run("claude_settings_conflict", func(t *testing.T) {
		dxac42ClaudeSettingsConflict(t, ctx, bin, podmanPath, fakeBinDir)
	})
}

func dxac42PiEnvConflict(t *testing.T, ctx context.Context, bin, podmanPath, fakeBinDir string) {
	t.Helper()
	piDir := t.TempDir()
	binDir := t.TempDir()
	otherCA := filepath.Join(t.TempDir(), "other-ca.pem")
	if err := os.WriteFile(otherCA, []byte("fake cert"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := dxh.AddRealProgramLink(binDir, "podman", podmanPath); err != nil {
		t.Fatal(err)
	}
	if err := dxh.AddRealProgramLink(binDir, "pi", filepath.Join(fakeBinDir, "pi")); err != nil {
		t.Fatal(err)
	}
	if err := dxh.AddRealProgramLink(binDir, "claude", filepath.Join(fakeBinDir, "claude")); err != nil {
		t.Fatal(err)
	}

	env := dxac42Env(t, piDir, binDir)
	env = append(env, "NODE_EXTRA_CA_CERTS="+otherCA)
	env = append(env, "SHELL=/bin/zsh")
	env = append(env, "ZDOTDIR="+t.TempDir())

	code, out := dxac42RunSetsid(ctx, bin, env, "agent", "setup", "pi",
		"--shell-profile", "yes", "--guard", "yes")
	if code != 1 {
		t.Fatalf("DXAC42 pi-env: expected exit 1; got %d\n%s", code, out)
	}
	if !strings.Contains(out, "bundle") {
		t.Errorf("DXAC42 pi-env: output does not contain bundle command; got %q", out)
	}
	if _, err := os.Stat(filepath.Join(piDir, "mcp.json")); !os.IsNotExist(err) {
		t.Error("DXAC42 pi-env: mcp.json was created despite conflict")
	}
}

func dxac42ClaudeSettingsConflict(t *testing.T, ctx context.Context, bin, podmanPath, fakeBinDir string) {
	t.Helper()
	claudeDir := t.TempDir()
	binDir := t.TempDir()
	otherCA := filepath.Join(t.TempDir(), "other-ca.pem")
	if err := os.WriteFile(otherCA, []byte("fake cert"), 0o600); err != nil {
		t.Fatal(err)
	}
	settings := filepath.Join(claudeDir, "settings.json")
	settingsContent := `{"env":{"NODE_EXTRA_CA_CERTS":"` + otherCA + `"}}`
	if err := os.WriteFile(settings, []byte(settingsContent), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := dxh.AddRealProgramLink(binDir, "podman", podmanPath); err != nil {
		t.Fatal(err)
	}
	if err := dxh.AddRealProgramLink(binDir, "claude", filepath.Join(fakeBinDir, "claude")); err != nil {
		t.Fatal(err)
	}

	env := dxac42EnvClaude(t, claudeDir, binDir)

	code, out := dxac42RunSetsid(ctx, bin, env, "agent", "setup", "claude-code",
		"--deny-rules", "yes")
	if code != 1 {
		t.Fatalf("DXAC42 claude-settings: expected exit 1; got %d\n%s", code, out)
	}
	if !strings.Contains(out, "bundle") {
		t.Errorf("DXAC42 claude-settings: output does not contain bundle command; got %q", out)
	}
	if _, err := os.Stat(filepath.Join(claudeDir, ".claude.json")); !os.IsNotExist(err) {
		t.Error("DXAC42 claude-settings: .claude.json was created despite conflict")
	}
}

func dxac42Env(t *testing.T, piDir, binDir string) []string {
	t.Helper()
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
	env = append(env, "CLAUDE_CONFIG_DIR="+t.TempDir())
	env = append(env, "PI_CODING_AGENT_DIR="+piDir)
	env = append(env, "PATH="+dxh.TestPATH(binDir))
	return env
}

func dxac42EnvClaude(t *testing.T, claudeDir, binDir string) []string {
	t.Helper()
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
	env = append(env, "PI_CODING_AGENT_DIR="+t.TempDir())
	env = append(env, "PATH="+dxh.TestPATH(binDir))
	return env
}

func dxac42RunSetsid(ctx context.Context, bin string, env []string, args ...string) (int, string) {
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
