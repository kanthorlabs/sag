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
	"time"

	"github.com/kanthorlabs/sag/e2e/devx/dxh"
)

func init() {
	agentTests["DXAC41_PiSetup"] = func(t *testing.T, ctx context.Context) {
		DXAC41_PiSetup(t, ctx)
	}
}

func DXAC41_PiSetup(t *testing.T, ctx context.Context) {
	t.Helper()
	dxh.EnsureBootstrapped(t)

	bin, err := dxh.TestBinary()
	if err != nil {
		t.Fatalf("DXAC41: test binary: %v", err)
	}
	podmanPath, err := exec.LookPath("podman")
	if err != nil {
		t.Fatalf("DXAC41: find podman: %v", err)
	}

	binDir := t.TempDir()
	if err := dxh.AddRealProgramLink(binDir, "podman", podmanPath); err != nil {
		t.Fatalf("DXAC41: link podman: %v", err)
	}

	fakeBinDir := dxh.ActiveAgentBinDir
	if fakeBinDir == "" {
		t.Skip("DXAC41: ActiveAgentBinDir not set; EnsureAgentSetup depends on D6.2 fakeharness")
	}
	if err := dxh.AddRealProgramLink(binDir, "pi", filepath.Join(fakeBinDir, "pi")); err != nil {
		t.Fatalf("DXAC41: link pi stub: %v", err)
	}
	if err := dxh.AddRealProgramLink(binDir, "claude", filepath.Join(fakeBinDir, "claude")); err != nil {
		t.Fatalf("DXAC41: link claude stub: %v", err)
	}

	home := os.Getenv("HOME")
	cfgDir := dxh.CfgDir(home)

	t.Run("bash_case", func(t *testing.T) {
		dxac41BashCase(t, ctx, bin, binDir, cfgDir)
	})

	t.Run("zsh_case", func(t *testing.T) {
		dxac41ZshCase(t, ctx, bin, binDir, cfgDir)
	})

	t.Run("second_run_ok_only", func(t *testing.T) {
		dxac41SecondRunOkOnly(t, ctx, bin, binDir, cfgDir)
	})
}

func dxac41BashCase(t *testing.T, ctx context.Context, bin, binDir, cfgDir string) {
	t.Helper()

	testHome := dxac41BuildTestHome(t, cfgDir)
	piDir := t.TempDir()

	env := dxac41BaseEnv(piDir, binDir)
	for i, e := range env {
		if strings.HasPrefix(e, "HOME=") {
			env[i] = "HOME=" + testHome
			break
		}
	}
	env = append(env, "SHELL=/bin/bash")

	code, out := dxac41RunSetsid(ctx, bin, env, "agent", "setup", "pi",
		"--shell-profile", "yes", "--guard", "yes")
	if code != 0 {
		t.Fatalf("DXAC41 bash: agent setup pi: exit %d\n%s", code, out)
	}

	bashProfile := filepath.Join(testHome, ".bash_profile")
	data, err := os.ReadFile(bashProfile)
	if err != nil {
		t.Fatalf("DXAC41 bash: read .bash_profile: %v", err)
	}
	exportLine := `export NODE_EXTRA_CA_CERTS="` + filepath.Join(cfgDir, "ca.pem") + `"`
	if !strings.Contains(string(data), exportLine) {
		t.Errorf("DXAC41 bash: .bash_profile does not contain export line; got %q", data)
	}
	if strings.Count(string(data), exportLine) != 1 {
		t.Errorf("DXAC41 bash: export line appears %d times; want 1", strings.Count(string(data), exportLine))
	}

	mcp := dxac41ReadMCP(t, filepath.Join(piDir, "mcp.json"))
	dxac41AssertPiMCP(t, mcp)

	guardPath := filepath.Join(piDir, "extensions", "sag-guard.ts")
	if _, err := os.Stat(guardPath); err != nil {
		t.Fatalf("DXAC41 bash: sag-guard.ts not created: %v", err)
	}
}

func dxac41BuildTestHome(t *testing.T, cfgDir string) string {
	t.Helper()
	testHome, err := os.MkdirTemp("", "sag-dxac41-home-*")
	if err != nil {
		t.Fatalf("DXAC41: create test home: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(testHome) })

	configDir := filepath.Join(testHome, ".config")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatalf("DXAC41: mkdir .config: %v", err)
	}
	if err := os.Symlink(cfgDir, filepath.Join(configDir, "sag")); err != nil {
		t.Fatalf("DXAC41: symlink .config/sag: %v", err)
	}

	localShareContainers := filepath.Join(testHome, ".local", "share", "containers", "podman")
	if err := os.MkdirAll(filepath.Dir(localShareContainers), 0o700); err != nil {
		t.Fatalf("DXAC41: mkdir .local/share/containers: %v", err)
	}
	realLocalShare := filepath.Join(os.Getenv("HOME"), ".local", "share", "containers", "podman")
	if _, err := os.Stat(realLocalShare); err == nil {
		if err := os.Symlink(realLocalShare, localShareContainers); err != nil {
			t.Fatalf("DXAC41: symlink podman machine: %v", err)
		}
	}

	configContainers := filepath.Join(testHome, ".config", "containers")
	realConfigContainers := filepath.Join(os.Getenv("HOME"), ".config", "containers")
	if _, err := os.Stat(realConfigContainers); err == nil {
		if err := os.Symlink(realConfigContainers, configContainers); err != nil {
			t.Fatalf("DXAC41: symlink .config/containers: %v", err)
		}
	}

	testCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	r := dxh.NewRunner()
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", testHome)
	defer os.Setenv("HOME", oldHome)
	if _, err := r.Output(testCtx, "podman", "ps", "--quiet"); err != nil {
		t.Fatalf("DXAC41 D6-a: podman ps from test home failed: %v; test home: %s; stop: cannot fall back to real home", err, testHome)
	}

	return testHome
}

func dxac41ZshCase(t *testing.T, ctx context.Context, bin, binDir, cfgDir string) {
	t.Helper()
	piDir := t.TempDir()
	zdotdir := t.TempDir()

	env := dxac41BaseEnv(piDir, binDir)
	env = append(env, "SHELL=/bin/zsh")
	env = append(env, "ZDOTDIR="+zdotdir)

	code, out := dxac41RunSetsid(ctx, bin, env, "agent", "setup", "pi",
		"--shell-profile", "yes", "--guard", "yes")
	if code != 0 {
		t.Fatalf("DXAC41 zsh: agent setup pi: exit %d\n%s", code, out)
	}

	zshrc := filepath.Join(zdotdir, ".zshrc")
	data, err := os.ReadFile(zshrc)
	if err != nil {
		t.Fatalf("DXAC41 zsh: read $ZDOTDIR/.zshrc: %v", err)
	}
	exportLine := `export NODE_EXTRA_CA_CERTS="` + filepath.Join(cfgDir, "ca.pem") + `"`
	if !strings.Contains(string(data), exportLine) {
		t.Errorf("DXAC41 zsh: .zshrc does not contain export line; got %q", data)
	}

	mcp := dxac41ReadMCP(t, filepath.Join(piDir, "mcp.json"))
	dxac41AssertPiMCP(t, mcp)
}

func dxac41SecondRunOkOnly(t *testing.T, ctx context.Context, bin, binDir, cfgDir string) {
	t.Helper()
	piDir := t.TempDir()
	zdotdir := t.TempDir()

	env := dxac41BaseEnv(piDir, binDir)
	env = append(env, "SHELL=/bin/zsh")
	env = append(env, "ZDOTDIR="+zdotdir)

	code, out := dxac41RunSetsid(ctx, bin, env, "agent", "setup", "pi",
		"--shell-profile", "yes", "--guard", "yes")
	if code != 0 {
		t.Fatalf("DXAC41 second-run: first run: exit %d\n%s", code, out)
	}

	code2, out2 := dxac41RunSetsid(ctx, bin, env, "agent", "setup", "pi")
	if code2 != 0 {
		t.Fatalf("DXAC41 second-run: second run: exit %d\n%s", code2, out2)
	}
	for _, line := range strings.Split(strings.TrimSpace(out2), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "ok ") {
			t.Errorf("DXAC41 second-run: non-ok line in output: %q", line)
		}
	}
}

func dxac41BaseEnv(piDir, binDir string) []string {
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
	env = append(env, "CLAUDE_CONFIG_DIR="+t_tmpDir41())
	env = append(env, "PI_CODING_AGENT_DIR="+piDir)
	env = append(env, "PATH="+dxh.TestPATH(binDir))
	return env
}

func dxac41RunSetsid(ctx context.Context, bin string, env []string, args ...string) (int, string) {
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

func dxac41ReadMCP(t *testing.T, mcpFile string) map[string]interface{} {
	t.Helper()
	data, err := os.ReadFile(mcpFile)
	if err != nil {
		t.Fatalf("DXAC41: read mcp.json: %v", err)
	}
	var top map[string]interface{}
	if err := json.Unmarshal(data, &top); err != nil {
		t.Fatalf("DXAC41: parse mcp.json: %v", err)
	}
	return top
}

func dxac41AssertPiMCP(t *testing.T, top map[string]interface{}) {
	t.Helper()
	servers, ok := top["mcpServers"].(map[string]interface{})
	if !ok {
		t.Fatal("DXAC41: mcpServers missing or wrong type")
	}
	sag, ok := servers["sag"].(map[string]interface{})
	if !ok {
		t.Fatal("DXAC41: mcpServers.sag missing or wrong type")
	}
	if sag["url"] == "" {
		t.Error("DXAC41: mcpServers.sag.url is empty")
	}
	if _, hasCmd := sag["command"]; hasCmd {
		t.Error("DXAC41: mcpServers.sag has command key; must not")
	}
	if sag["exposure"] != "direct" {
		t.Errorf("DXAC41: mcpServers.sag.exposure = %v; want 'direct'", sag["exposure"])
	}
}

func t_tmpDir41() string {
	dir, _ := os.MkdirTemp("", "sag-dxac41-claude-*")
	return dir
}
