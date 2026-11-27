//go:build devxe2e

package devx_test

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
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
	testHome := t.TempDir()
	piDir := t.TempDir()
	binDir := t.TempDir()

	cfgDir := dxh.CfgDir(testHome)
	if err := os.MkdirAll(cfgDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "ca.pem"), []byte("fake-ca-pem"), 0o600); err != nil {
		t.Fatal(err)
	}

	otherCA := filepath.Join(t.TempDir(), "other-ca.pem")
	if err := os.WriteFile(otherCA, []byte("other-ca-pem"), 0o600); err != nil {
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

	env := dxac42EnvWithHome(t, testHome, piDir, binDir)
	env = append(env, "NODE_EXTRA_CA_CERTS="+otherCA)
	env = append(env, "SHELL=/bin/zsh")
	zdotdir := t.TempDir()
	env = append(env, "ZDOTDIR="+zdotdir)

	snapBefore := dxac42Snapshot(piDir, zdotdir)

	code, out := dxac42RunSetsid(ctx, bin, env, "agent", "setup", "pi",
		"--shell-profile", "yes", "--guard", "yes")
	if code != 1 {
		t.Fatalf("DXAC42 pi-env: expected exit 1; got %d\n%s", code, out)
	}
	if !strings.Contains(out, "bundle") {
		t.Errorf("DXAC42 pi-env: output does not contain bundle command; got %q", out)
	}

	snapAfter := dxac42Snapshot(piDir, zdotdir)
	dxac42AssertUnchanged(t, "pi-env conflict", snapBefore, snapAfter)

	bundleCmd := dxac42ParseBundleCmd(out)
	if bundleCmd == "" {
		t.Log("DXAC42 pi-env: bundle command not found in output (expected until D6.1 merges)")
		return
	}
	if err := dxac42RunBundleCmd(bundleCmd); err != nil {
		t.Fatalf("DXAC42 pi-env: run bundle command: %v", err)
	}
	bundlePath := filepath.Join(cfgDir, "ca-bundle.pem")
	if _, err := os.Stat(bundlePath); err != nil {
		t.Fatalf("DXAC42 pi-env: bundle file not created at %s: %v", bundlePath, err)
	}

	env2 := dxac42EnvWithHome(t, testHome, piDir, binDir)
	env2 = append(env2, "NODE_EXTRA_CA_CERTS="+bundlePath)
	env2 = append(env2, "SHELL=/bin/zsh")
	env2 = append(env2, "ZDOTDIR="+zdotdir)
	code2, out2 := dxac42RunSetsid(ctx, bin, env2, "agent", "setup", "pi",
		"--shell-profile", "yes", "--guard", "yes")
	if code2 != 0 {
		t.Fatalf("DXAC42 pi-env: after bundle, expected exit 0; got %d\n%s", code2, out2)
	}
}

func dxac42ClaudeSettingsConflict(t *testing.T, ctx context.Context, bin, podmanPath, fakeBinDir string) {
	t.Helper()
	testHome := t.TempDir()
	claudeDir := t.TempDir()
	binDir := t.TempDir()

	cfgDir := dxh.CfgDir(testHome)
	if err := os.MkdirAll(cfgDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "ca.pem"), []byte("fake-ca-pem"), 0o600); err != nil {
		t.Fatal(err)
	}

	otherCA := filepath.Join(t.TempDir(), "other-ca.pem")
	if err := os.WriteFile(otherCA, []byte("other-ca-pem"), 0o600); err != nil {
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

	env := dxac42EnvClaudeWithHome(t, testHome, claudeDir, binDir)

	snapBefore := dxac42Snapshot(claudeDir)

	code, out := dxac42RunSetsid(ctx, bin, env, "agent", "setup", "claude-code",
		"--deny-rules", "yes")
	if code != 1 {
		t.Fatalf("DXAC42 claude-settings: expected exit 1; got %d\n%s", code, out)
	}
	if !strings.Contains(out, "bundle") {
		t.Errorf("DXAC42 claude-settings: output does not contain bundle command; got %q", out)
	}

	snapAfter := dxac42Snapshot(claudeDir)
	dxac42AssertUnchanged(t, "claude-settings conflict", snapBefore, snapAfter)

	bundleCmd := dxac42ParseBundleCmd(out)
	if bundleCmd == "" {
		t.Log("DXAC42 claude-settings: bundle command not found in output (expected until D6.1 merges)")
		return
	}
	if err := dxac42RunBundleCmd(bundleCmd); err != nil {
		t.Fatalf("DXAC42 claude-settings: run bundle command: %v", err)
	}
	bundlePath := filepath.Join(cfgDir, "ca-bundle.pem")
	if _, err := os.Stat(bundlePath); err != nil {
		t.Fatalf("DXAC42 claude-settings: bundle file not created at %s: %v", bundlePath, err)
	}

	env2 := dxac42EnvClaudeWithHome(t, testHome, claudeDir, binDir)
	settings2Content := `{"env":{"NODE_EXTRA_CA_CERTS":"` + bundlePath + `"}}`
	if err := os.WriteFile(settings, []byte(settings2Content), 0o640); err != nil {
		t.Fatalf("DXAC42 claude-settings: rewrite settings: %v", err)
	}
	code2, out2 := dxac42RunSetsid(ctx, bin, env2, "agent", "setup", "claude-code",
		"--deny-rules", "yes")
	if code2 != 0 {
		t.Fatalf("DXAC42 claude-settings: after bundle, expected exit 0; got %d\n%s", code2, out2)
	}
}

func dxac42Snapshot(dirs ...string) map[string][]byte {
	out := map[string][]byte{}
	for _, dir := range dirs {
		filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			data, err := os.ReadFile(path)
			if err == nil {
				out[path] = data
			}
			return nil
		})
	}
	return out
}

func dxac42AssertUnchanged(t *testing.T, label string, before, after map[string][]byte) {
	t.Helper()
	for path, bData := range before {
		aData, ok := after[path]
		if !ok {
			continue
		}
		if !bytes.Equal(bData, aData) {
			t.Errorf("DXAC42 %s: file modified: %s", label, path)
		}
	}
	for path := range after {
		if _, existed := before[path]; !existed {
			t.Errorf("DXAC42 %s: new file created: %s", label, path)
		}
	}
}

func dxac42ParseBundleCmd(output string) string {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "cat ") && strings.Contains(line, "ca.pem") {
			return line
		}
	}
	return ""
}

func dxac42RunBundleCmd(cmd string) error {
	c := exec.Command("sh", "-c", cmd)
	out, err := c.CombinedOutput()
	if err != nil {
		return fmt.Errorf("bundle cmd %q: %w\n%s", cmd, err, out)
	}
	return nil
}

func dxac42EnvWithHome(t *testing.T, home, piDir, binDir string) []string {
	t.Helper()
	var env []string
	for _, e := range os.Environ() {
		key, _, _ := strings.Cut(e, "=")
		switch key {
		case "TMPDIR", "USER", "LANG":
			env = append(env, e)
		default:
			if strings.HasPrefix(key, "SAG_TEST_") {
				env = append(env, e)
			}
		}
	}
	env = append(env, "HOME="+home)
	env = append(env, "CLAUDE_CONFIG_DIR="+t.TempDir())
	env = append(env, "PI_CODING_AGENT_DIR="+piDir)
	env = append(env, "PATH="+dxh.TestPATH(binDir))
	return env
}

func dxac42EnvClaudeWithHome(t *testing.T, home, claudeDir, binDir string) []string {
	t.Helper()
	var env []string
	for _, e := range os.Environ() {
		key, _, _ := strings.Cut(e, "=")
		switch key {
		case "TMPDIR", "USER", "LANG":
			env = append(env, e)
		default:
			if strings.HasPrefix(key, "SAG_TEST_") {
				env = append(env, e)
			}
		}
	}
	env = append(env, "HOME="+home)
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
