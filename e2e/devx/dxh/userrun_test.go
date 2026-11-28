package dxh_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kanthorlabs/sag/e2e/devx/dxh"
)

func TestRefusesClaudeProcess(t *testing.T) {
	psOut := "user   123   0.0  0.0  4096  512 ??  S   Mon01PM   0:00.00 /usr/local/bin/claude arg1\n"
	err := dxh.ScanForClaudeProcess(psOut)
	if err == nil {
		t.Fatal("ScanForClaudeProcess: expected error for running claude process, got nil")
	}
	if !strings.Contains(err.Error(), "claude") {
		t.Errorf("ScanForClaudeProcess: error does not mention claude: %v", err)
	}
}

func TestRefusesNodeClaudeProcess(t *testing.T) {
	psOut := "user   456   0.0  0.0  4096  512 ??  S   Mon01PM   0:00.00 /usr/bin/node /home/user/.claude/cli/main.js\n"
	err := dxh.ScanForClaudeProcess(psOut)
	if err == nil {
		t.Fatal("ScanForClaudeProcess: expected error for node process with claude arg, got nil")
	}
	if !strings.Contains(err.Error(), "node") {
		t.Errorf("ScanForClaudeProcess: error does not mention node: %v", err)
	}
}

func TestScanForClaudeProcessIgnoresNonClaude(t *testing.T) {
	psOut := "USER   PID %CPU %MEM      VSZ    RSS  TT  STAT STARTED      TIME COMMAND\n" +
		"user   789   0.0  0.0  4096  512 ??  S   Mon01PM   0:00.00 /usr/bin/node server.js\n" +
		"user   101   0.0  0.0  4096  512 ??  S   Mon01PM   0:00.00 /usr/bin/python3 script.py\n"
	err := dxh.ScanForClaudeProcess(psOut)
	if err != nil {
		t.Fatalf("ScanForClaudeProcess: unexpected error for non-claude processes: %v", err)
	}
}

func TestRefusesClaudeConfigDir(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_DIR", "/tmp/test-claude")
	t.Setenv("PI_CODING_AGENT_DIR", "")
	err := dxh.CheckProfileEnv()
	if err == nil {
		t.Fatal("CheckProfileEnv: expected error for CLAUDE_CONFIG_DIR set, got nil")
	}
	if !strings.Contains(err.Error(), "CLAUDE_CONFIG_DIR") {
		t.Errorf("CheckProfileEnv: error does not mention CLAUDE_CONFIG_DIR: %v", err)
	}
	if !strings.Contains(err.Error(), "unset CLAUDE_CONFIG_DIR") {
		t.Errorf("CheckProfileEnv: error does not contain 'unset CLAUDE_CONFIG_DIR': %v", err)
	}
}

func TestRefusesPiCodingAgentDir(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	t.Setenv("PI_CODING_AGENT_DIR", "/tmp/test-pi")
	err := dxh.CheckProfileEnv()
	if err == nil {
		t.Fatal("CheckProfileEnv: expected error for PI_CODING_AGENT_DIR set, got nil")
	}
	if !strings.Contains(err.Error(), "PI_CODING_AGENT_DIR") {
		t.Errorf("CheckProfileEnv: error does not mention PI_CODING_AGENT_DIR: %v", err)
	}
	if !strings.Contains(err.Error(), "unset PI_CODING_AGENT_DIR") {
		t.Errorf("CheckProfileEnv: error does not contain 'unset PI_CODING_AGENT_DIR': %v", err)
	}
}

func TestCheckProfileEnvPassesWhenUnset(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	t.Setenv("PI_CODING_AGENT_DIR", "")
	err := dxh.CheckProfileEnv()
	if err != nil {
		t.Fatalf("CheckProfileEnv: unexpected error when env vars unset: %v", err)
	}
}

func TestTeardownHarnessSteps(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("SHELL", "/bin/zsh")
	t.Setenv("ZDOTDIR", "")

	claudeDir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(filepath.Join(claudeDir, "skills", "sag"), 0o700); err != nil {
		t.Fatal(err)
	}
	settingsPath := filepath.Join(claudeDir, "settings.json")
	settings := map[string]interface{}{
		"env": map[string]string{"NODE_EXTRA_CA_CERTS": "/path/ca.pem"},
		"permissions": map[string]interface{}{
			"deny": []string{"Bash(sag credential delete:*)", "Bash(podman secret rm:*)", "Bash(script:*)"},
		},
		"other": "value",
	}
	writeJSON(t, settingsPath, settings)

	piDir := filepath.Join(home, ".pi")
	if err := os.MkdirAll(filepath.Join(piDir, "skills", "sag"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(piDir, "extensions"), 0o700); err != nil {
		t.Fatal(err)
	}
	guardPath := filepath.Join(piDir, "extensions", "sag-guard.ts")
	if err := os.WriteFile(guardPath, []byte("// managed by sag agent setup\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	zshrc := filepath.Join(home, ".zshrc")
	if err := os.WriteFile(zshrc, []byte("# user stuff\n# sag agent setup\nexport NODE_EXTRA_CA_CERTS=\"/path/ca.pem\"\n# more user stuff\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	f := newFake()
	f.set("claude mcp remove sag -s user", []byte("ok\n"), nil)
	f.set("pi mcp remove sag", []byte("ok\n"), nil)

	if err := dxh.TeardownHarnessFiles(context.Background(), f, home); err != nil {
		t.Fatalf("TeardownHarnessFiles: %v", err)
	}

	found := false
	for _, call := range f.calls {
		if strings.Join(call, " ") == "claude mcp remove sag -s user" {
			found = true
		}
	}
	if !found {
		t.Error("TeardownHarnessFiles: did not call 'claude mcp remove sag -s user'")
	}
	found = false
	for _, call := range f.calls {
		if strings.Join(call, " ") == "pi mcp remove sag" {
			found = true
		}
	}
	if !found {
		t.Error("TeardownHarnessFiles: did not call 'pi mcp remove sag'")
	}

	if _, err := os.Stat(filepath.Join(claudeDir, "skills", "sag")); !os.IsNotExist(err) {
		t.Error("TeardownHarnessFiles: claude skills/sag not removed")
	}
	if _, err := os.Stat(filepath.Join(piDir, "skills", "sag")); !os.IsNotExist(err) {
		t.Error("TeardownHarnessFiles: pi skills/sag not removed")
	}
	if _, err := os.Stat(guardPath); !os.IsNotExist(err) {
		t.Error("TeardownHarnessFiles: pi extensions/sag-guard.ts not removed")
	}

	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]interface{}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if env, ok := got["env"].(map[string]interface{}); ok {
		if _, has := env["NODE_EXTRA_CA_CERTS"]; has {
			t.Error("TeardownHarnessFiles: NODE_EXTRA_CA_CERTS not removed from settings.json")
		}
	}

	zshrcData, err := os.ReadFile(zshrc)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(zshrcData), "# sag agent setup") {
		t.Error("TeardownHarnessFiles: '# sag agent setup' line not removed from .zshrc")
	}
	if strings.Contains(string(zshrcData), "NODE_EXTRA_CA_CERTS") {
		t.Error("TeardownHarnessFiles: NODE_EXTRA_CA_CERTS not removed from .zshrc")
	}
	if !strings.Contains(string(zshrcData), "# user stuff") {
		t.Error("TeardownHarnessFiles: non-SAG content removed from .zshrc")
	}
}

func TestAbsentFileSnapshot(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("SHELL", "/bin/zsh")
	t.Setenv("ZDOTDIR", "")

	snapDir := t.TempDir()
	snap, err := dxh.Snapshot(snapDir)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}

	if !snap.Absent["claude.json"] {
		t.Error("Snapshot: missing ~/.claude.json not recorded as absent")
	}
	if !snap.Absent["claude-settings.json"] {
		t.Error("Snapshot: missing ~/.claude/settings.json not recorded as absent")
	}
	if !snap.Absent["pi-mcp.json"] {
		t.Error("Snapshot: missing ~/.pi/agent/mcp.json not recorded as absent")
	}
}

func TestCompareSAGEntriesOnly(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("SHELL", "/bin/zsh")
	t.Setenv("ZDOTDIR", "")

	claudeJSON := filepath.Join(home, ".claude.json")
	writeJSON(t, claudeJSON, map[string]interface{}{
		"mcpServers": map[string]interface{}{},
		"other_key":  "original_value",
	})

	snapDir := t.TempDir()
	snap, err := dxh.Snapshot(snapDir)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}

	writeJSON(t, claudeJSON, map[string]interface{}{
		"mcpServers": map[string]interface{}{},
		"other_key":  "changed_value",
	})

	if err := dxh.CompareSAGEntries(snap); err != nil {
		t.Errorf("CompareSAGEntries: non-SAG change caused failure: %v", err)
	}
}

func TestDifferenceKeepsCopy(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("SHELL", "/bin/zsh")
	t.Setenv("ZDOTDIR", "")

	claudeJSON := filepath.Join(home, ".claude.json")
	writeJSON(t, claudeJSON, map[string]interface{}{
		"mcpServers": map[string]interface{}{},
	})

	snapDir := t.TempDir()
	snap, err := dxh.Snapshot(snapDir)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}

	writeJSON(t, claudeJSON, map[string]interface{}{
		"mcpServers": map[string]interface{}{
			"sag": map[string]interface{}{"type": "http", "url": "https://localhost:8443/mcp"},
		},
	})

	if err := dxh.CompareSAGEntries(snap); err == nil {
		t.Fatal("CompareSAGEntries: expected error on SAG entry difference, got nil")
	}

	if _, err := os.Stat(snapDir); err != nil {
		t.Errorf("CompareSAGEntries: copy directory removed on difference: %v", err)
	}
}

func TestNoRestore(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("SHELL", "/bin/zsh")
	t.Setenv("ZDOTDIR", "")

	claudeJSON := filepath.Join(home, ".claude.json")
	original := map[string]interface{}{
		"mcpServers": map[string]interface{}{},
		"some_key":   "some_value",
	}
	writeJSON(t, claudeJSON, original)

	snapDir := t.TempDir()
	snap, err := dxh.Snapshot(snapDir)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}

	modified := map[string]interface{}{
		"mcpServers": map[string]interface{}{
			"sag": map[string]interface{}{"type": "http", "url": "https://localhost:8443/mcp"},
		},
		"some_key": "changed",
	}
	writeJSON(t, claudeJSON, modified)

	_ = dxh.CompareSAGEntries(snap)

	data, err := os.ReadFile(claudeJSON)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]interface{}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	mcpServers, _ := got["mcpServers"].(map[string]interface{})
	if _, hasSAG := mcpServers["sag"]; !hasSAG {
		t.Error("CompareSAGEntries: restored file (wrote mcpServers.sag removal); CompareSAGEntries must not restore files")
	}
}

func TestCopyMode0700(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("SHELL", "/bin/zsh")
	t.Setenv("ZDOTDIR", "")

	snapDir := filepath.Join(t.TempDir(), "snap")
	_, err := dxh.Snapshot(snapDir)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}

	info, err := os.Stat(snapDir)
	if err != nil {
		t.Fatalf("os.Stat snapshot dir: %v", err)
	}
	if info.Mode().Perm() != 0o700 {
		t.Errorf("Snapshot: directory mode = %04o; want 0700", info.Mode().Perm())
	}
}

func TestMasksTailscaleIP(t *testing.T) {
	line := "output: 100.1.2.3 connected to server 100.1.2.3"
	got := dxh.MaskTailscaleIP(line, "100.1.2.3")
	want := "output: <tailscale IP> connected to server <tailscale IP>"
	if got != want {
		t.Errorf("MaskTailscaleIP: got %q; want %q", got, want)
	}
}

func TestMasksTailscaleIPEmptyIP(t *testing.T) {
	line := "output: 100.1.2.3 connected"
	got := dxh.MaskTailscaleIP(line, "")
	if got != line {
		t.Errorf("MaskTailscaleIP with empty IP changed line: got %q; want %q", got, line)
	}
}

func writeJSON(t *testing.T, path string, v interface{}) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}
