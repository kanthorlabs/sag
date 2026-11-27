package agentsetup

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func baseOpts(t *testing.T) Options {
	t.Helper()
	dir := t.TempDir()
	return Options{
		Harness:    HarnessClaudeCode,
		Sub:        "claude-code",
		URL:        "https://localhost:8443/mcp",
		CfgDir:     dir,
		PodmanPath: "podman",
		Env: map[string]string{
			"CLAUDE_CONFIG_DIR": dir,
			"PATH":              "/usr/bin:/bin",
		},
		Home: dir,
		GOOS: "darwin",
	}
}

func hasConflict(conflicts []string, substr string) bool {
	for _, c := range conflicts {
		if containsString(c, substr) {
			return true
		}
	}
	return false
}

func TestConflictHarnessNotInPATH(t *testing.T) {
	opts := baseOpts(t)
	conflicts := planConflicts(opts, ModeLocal, true)
	if !hasConflict(conflicts, "not in PATH") {
		t.Errorf("conflicts = %v; want 'not in PATH' conflict", conflicts)
	}
}

func TestConflictContainerNotRunning(t *testing.T) {
	opts := baseOpts(t)
	conflicts := planConflicts(opts, ModeLocal, false)
	if !hasConflict(conflicts, "does not run") {
		t.Errorf("conflicts = %v; want 'does not run' conflict", conflicts)
	}
}

func TestConflictContainerNotRunningSkippedWhenBootstrap(t *testing.T) {
	opts := baseOpts(t)
	opts.UnderLock = true
	conflicts := planConflicts(opts, ModeLocal, false)
	if hasConflict(conflicts, "does not run") {
		t.Error("conflicts should not include 'does not run' when under bootstrap lock")
	}
}

func TestConflictRemoteMissingCAPem(t *testing.T) {
	opts := baseOpts(t)
	opts.URL = "https://remote.example:8443/mcp"
	conflicts := planConflicts(opts, ModeRemote, false)
	if !hasConflict(conflicts, "ca.pem absent") {
		t.Errorf("conflicts = %v; want 'ca.pem absent' conflict for remote mode", conflicts)
	}
}

func TestConflictInvalidMCPFile(t *testing.T) {
	opts := baseOpts(t)
	dir := opts.Env["CLAUDE_CONFIG_DIR"]
	mcpFile := filepath.Join(dir, ".claude.json")
	os.WriteFile(mcpFile, []byte("not json"), 0o600)
	conflicts := planConflicts(opts, ModeLocal, true)
	if !hasConflict(conflicts, "not valid JSON") {
		t.Errorf("conflicts = %v; want 'not valid JSON' conflict", conflicts)
	}
}

func TestConflictCommandEntry(t *testing.T) {
	opts := baseOpts(t)
	dir := opts.Env["CLAUDE_CONFIG_DIR"]
	mcpFile := filepath.Join(dir, ".claude.json")
	entry := map[string]any{
		"mcpServers": map[string]any{
			"sag": map[string]any{
				"command": "sag",
				"url":     "https://localhost:8443/mcp",
			},
		},
	}
	data, _ := json.Marshal(entry)
	os.WriteFile(mcpFile, data, 0o600)
	conflicts := planConflicts(opts, ModeLocal, true)
	if !hasConflict(conflicts, "another form") {
		t.Errorf("conflicts = %v; want 'another form' conflict for command entry", conflicts)
	}
}

func TestConflictSkillWithoutMarker(t *testing.T) {
	opts := baseOpts(t)
	dir := opts.Env["CLAUDE_CONFIG_DIR"]
	skillPath := filepath.Join(dir, "skills", "sag", "SKILL.md")
	os.MkdirAll(filepath.Dir(skillPath), 0o700)
	os.WriteFile(skillPath, []byte("# Custom skill\nno marker\n"), 0o644)
	conflicts := planConflicts(opts, ModeLocal, true)
	if !hasConflict(conflicts, "managed marker") {
		t.Errorf("conflicts = %v; want 'managed marker' conflict for skill without marker", conflicts)
	}
}

func TestConflictGuardWithoutMarker(t *testing.T) {
	opts := baseOpts(t)
	opts.Harness = HarnessPi
	piDir := t.TempDir()
	opts.Env["PI_CODING_AGENT_DIR"] = piDir
	guardPath := filepath.Join(piDir, "extensions", "sag-guard.ts")
	os.MkdirAll(filepath.Dir(guardPath), 0o700)
	os.WriteFile(guardPath, []byte("// custom guard\n"), 0o644)
	conflicts := planConflicts(opts, ModeLocal, true)
	if !hasConflict(conflicts, "managed marker") {
		t.Errorf("conflicts = %v; want 'managed marker' conflict for guard without marker", conflicts)
	}
}

func TestConflictFishWithShellProfile(t *testing.T) {
	opts := baseOpts(t)
	opts.Harness = HarnessPi
	opts.ShellProfile = "yes"
	opts.Env["SHELL"] = "/usr/local/bin/fish"
	piDir := t.TempDir()
	opts.Env["PI_CODING_AGENT_DIR"] = piDir
	conflicts := planConflicts(opts, ModeLocal, true)
	if !hasConflict(conflicts, "unsupported shell") {
		t.Errorf("conflicts = %v; want 'unsupported shell' conflict for fish with --shell-profile yes", conflicts)
	}
}
