//go:build devxe2e

package devx_test

import (
	"bufio"
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
		t.Fatal("DXAC41: dxh.ActiveAgentBinDir is not set")
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
		dxac41BashCase(t, ctx, bin, binDir, fakeBinDir, cfgDir)
	})

	t.Run("zsh_case", func(t *testing.T) {
		dxac41ZshCase(t, ctx, bin, binDir, fakeBinDir, cfgDir)
	})

	t.Run("second_run_ok_only", func(t *testing.T) {
		dxac41SecondRunOkOnly(t, ctx, bin, binDir, cfgDir)
	})
}

func dxac41BashCase(t *testing.T, ctx context.Context, bin, binDir, fakeBinDir, cfgDir string) {
	t.Helper()

	testHome := dxac41BuildTestHome(t, cfgDir)
	piDir := t.TempDir()

	env := dxac41BaseEnvForHome(t, testHome, piDir, binDir)
	env = append(env, "SHELL=/bin/bash")

	stubsBefore := dxac41CountStubRecords(fakeBinDir)
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
	testCfgDir := filepath.Join(testHome, ".config", "sag")
	exportLine := `export NODE_EXTRA_CA_CERTS="` + filepath.Join(testCfgDir, "ca.pem") + `"`
	profileStr := string(data)
	if !strings.Contains(profileStr, exportLine) {
		t.Errorf("DXAC41 bash: .bash_profile does not contain export line; got %q", profileStr)
	}
	if strings.Count(profileStr, exportLine) != 1 {
		t.Errorf("DXAC41 bash: export line appears %d times; want 1", strings.Count(profileStr, exportLine))
	}

	mcpPath := filepath.Join(piDir, "mcp.json")
	dxac41AssertMCPMode(t, mcpPath)
	mcp := dxac41ReadMCP(t, mcpPath)
	dxac41AssertPiMCP(t, "bash", mcp)
	dxac41AssertSkill(t, piDir, "bash")
	dxac41AssertGuardMarker(t, filepath.Join(piDir, "extensions", "sag-guard.ts"))
	dxac41AssertStubReceived(t, fakeBinDir, cfgDir, "bash", stubsBefore)

	stubsAfterFirst := dxac41CountStubRecords(fakeBinDir)
	code2, out2 := dxac41RunSetsid(ctx, bin, env, "agent", "setup", "pi")
	if code2 != 0 {
		t.Fatalf("DXAC41 bash: second run: exit %d\n%s", code2, out2)
	}
	dxac41AssertOnlyOkLines(t, "bash second run", out2)
	stubsAfterSecond := dxac41CountStubRecords(fakeBinDir)
	if stubsAfterSecond <= stubsAfterFirst {
		t.Errorf("DXAC41 bash: second run did not call stub pi; records before=%d after=%d", stubsAfterFirst, stubsAfterSecond)
	}
}

func dxac41BuildTestHome(t *testing.T, cfgDir string) string {
	t.Helper()
	testHome := t.TempDir()

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

func dxac41ZshCase(t *testing.T, ctx context.Context, bin, binDir, fakeBinDir, cfgDir string) {
	t.Helper()
	piDir := t.TempDir()
	zdotdir := t.TempDir()

	env := dxac41BaseEnv(t, piDir, binDir)
	env = append(env, "SHELL=/bin/zsh")
	env = append(env, "ZDOTDIR="+zdotdir)

	stubsBefore := dxac41CountStubRecords(fakeBinDir)
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
	profileStr := string(data)
	if !strings.Contains(profileStr, exportLine) {
		t.Errorf("DXAC41 zsh: .zshrc does not contain export line; got %q", profileStr)
	}
	if strings.Count(profileStr, exportLine) != 1 {
		t.Errorf("DXAC41 zsh: export line appears %d times; want 1", strings.Count(profileStr, exportLine))
	}

	mcpPath := filepath.Join(piDir, "mcp.json")
	dxac41AssertMCPMode(t, mcpPath)
	mcp := dxac41ReadMCP(t, mcpPath)
	dxac41AssertPiMCP(t, "zsh", mcp)
	dxac41AssertSkill(t, piDir, "zsh")
	dxac41AssertGuardMarker(t, filepath.Join(piDir, "extensions", "sag-guard.ts"))
	dxac41AssertStubReceived(t, fakeBinDir, cfgDir, "zsh", stubsBefore)

	code2, out2 := dxac41RunSetsid(ctx, bin, env, "agent", "setup", "pi")
	if code2 != 0 {
		t.Fatalf("DXAC41 zsh: second run: exit %d\n%s", code2, out2)
	}
	dxac41AssertOnlyOkLines(t, "zsh second run", out2)
}

func dxac41SecondRunOkOnly(t *testing.T, ctx context.Context, bin, binDir, cfgDir string) {
	t.Helper()
	piDir := t.TempDir()
	zdotdir := t.TempDir()

	env := dxac41BaseEnv(t, piDir, binDir)
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
	dxac41AssertOnlyOkLines(t, "second run", out2)
}

func dxac41AssertOnlyOkLines(t *testing.T, label, output string) {
	t.Helper()
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "ok ") {
			t.Errorf("DXAC41 %s: non-ok line: %q", label, line)
		}
	}
}

func dxac41AssertMCPMode(t *testing.T, mcpPath string) {
	t.Helper()
	info, err := os.Stat(mcpPath)
	if err != nil {
		t.Fatalf("DXAC41: stat mcp.json: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("DXAC41: mcp.json mode = %o; want 0600", info.Mode().Perm())
	}
}

func dxac41AssertPiMCP(t *testing.T, label string, top map[string]interface{}) {
	t.Helper()
	servers, ok := top["mcpServers"].(map[string]interface{})
	if !ok {
		t.Fatalf("DXAC41 %s: mcpServers missing or wrong type", label)
	}
	sag, ok := servers["sag"].(map[string]interface{})
	if !ok {
		t.Fatalf("DXAC41 %s: mcpServers.sag missing or wrong type", label)
	}
	url, _ := sag["url"].(string)
	if url == "" {
		t.Errorf("DXAC41 %s: mcpServers.sag.url is empty", label)
	}
	if _, hasCmd := sag["command"]; hasCmd {
		t.Errorf("DXAC41 %s: mcpServers.sag has command key; must not", label)
	}
	if sag["exposure"] != "direct" {
		t.Errorf("DXAC41 %s: mcpServers.sag.exposure = %v; want 'direct'", label, sag["exposure"])
	}
	headers, ok := sag["headers"].(map[string]interface{})
	if !ok {
		t.Errorf("DXAC41 %s: mcpServers.sag.headers missing or wrong type", label)
	} else {
		auth, _ := headers["Authorization"].(string)
		if !strings.HasPrefix(auth, "Bearer ") {
			t.Errorf("DXAC41 %s: mcpServers.sag.headers.Authorization = %q; want 'Bearer ...'", label, auth)
		}
	}
}

func dxac41AssertSkill(t *testing.T, piDir, label string) {
	t.Helper()
	skillPath := filepath.Join(piDir, "skills", "sag", "SKILL.md")
	data, err := os.ReadFile(skillPath)
	if err != nil {
		t.Fatalf("DXAC41 %s: read SKILL.md: %v", label, err)
	}
	content := string(data)
	if !strings.Contains(content, "<!-- managed by sag agent setup -->") {
		t.Errorf("DXAC41 %s: SKILL.md does not contain marker", label)
	}
	if !strings.Contains(content, "--agent pi") {
		t.Errorf("DXAC41 %s: SKILL.md does not contain '--agent pi'", label)
	}
}

func dxac41AssertGuardMarker(t *testing.T, guardPath string) {
	t.Helper()
	f, err := os.Open(guardPath)
	if err != nil {
		t.Fatalf("DXAC41: open sag-guard.ts: %v", err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	if !sc.Scan() {
		t.Fatal("DXAC41: sag-guard.ts is empty")
	}
	line1 := sc.Text()
	if line1 != "// managed by sag agent setup" {
		t.Errorf("DXAC41: sag-guard.ts line 1 = %q; want '// managed by sag agent setup'", line1)
	}
}

func dxac41CountStubRecords(fakeBinDir string) int {
	data, err := os.ReadFile(filepath.Join(fakeBinDir, "stub_records.json"))
	if err != nil {
		return 0
	}
	n := 0
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if strings.TrimSpace(line) != "" {
			n++
		}
	}
	return n
}

func dxac41AssertStubReceived(t *testing.T, fakeBinDir, cfgDir, label string, skipFirst int) {
	t.Helper()
	recordPath := filepath.Join(fakeBinDir, "stub_records.json")
	data, err := os.ReadFile(recordPath)
	if err != nil {
		t.Errorf("DXAC41 %s: read stub_records.json: %v", label, err)
		return
	}
	want := filepath.Join(cfgDir, "ca.pem")
	found := false
	idx := 0
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if line == "" {
			continue
		}
		if idx < skipFirst {
			idx++
			continue
		}
		idx++
		var rec struct {
			Argv []string          `json:"argv"`
			Env  map[string]string `json:"env"`
		}
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			continue
		}
		if len(rec.Argv) == 3 && rec.Argv[0] == "mcp" && rec.Argv[1] == "list" && rec.Argv[2] == "--json" {
			if rec.Env["NODE_EXTRA_CA_CERTS"] == want {
				found = true
				break
			}
		}
	}
	if !found {
		t.Errorf("DXAC41 %s: stub pi did not receive 'mcp list --json' with NODE_EXTRA_CA_CERTS=%q in records after offset %d", label, want, skipFirst)
	}
}

func dxac41BaseEnvForHome(t *testing.T, home, piDir, binDir string) []string {
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

func dxac41BaseEnv(t *testing.T, piDir, binDir string) []string {
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
