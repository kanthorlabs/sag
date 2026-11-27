package agentsetup

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestSkillInstallWritesFile(t *testing.T) {
	dir := t.TempDir()
	var out bytes.Buffer
	SetOut(&out)
	defer SetOut(os.Stdout)

	opts := Options{
		Harness:    HarnessClaudeCode,
		Sub:        "claude-code",
		URL:        "https://localhost:8443/mcp",
		CfgDir:     dir,
		PodmanPath: "podman",
		Env:        map[string]string{"CLAUDE_CONFIG_DIR": dir},
		Home:       dir,
	}
	if err := installSkill(context.Background(), opts); err != nil {
		t.Fatalf("installSkill: %v", err)
	}
	paths := ClaudeCodePaths(dir, dir)
	data, err := os.ReadFile(paths.SkillFile)
	if err != nil {
		t.Fatalf("read skill file: %v", err)
	}
	if !bytes.Contains(data, []byte(skillMarker)) {
		t.Error("skill file missing marker")
	}
}

func TestSkillInstallSkipsWithoutMarker(t *testing.T) {
	dir := t.TempDir()
	paths := ClaudeCodePaths(dir, dir)
	if err := os.MkdirAll(filepath.Dir(paths.SkillFile), 0o700); err != nil {
		t.Fatal(err)
	}
	original := []byte("# Custom skill\nno marker here\n")
	if err := os.WriteFile(paths.SkillFile, original, 0o644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	SetOut(&out)
	defer SetOut(os.Stdout)

	opts := Options{
		Harness:    HarnessClaudeCode,
		Sub:        "claude-code",
		URL:        "https://localhost:8443/mcp",
		CfgDir:     dir,
		PodmanPath: "podman",
		Env:        map[string]string{"CLAUDE_CONFIG_DIR": dir},
		Home:       dir,
	}
	if err := installSkill(context.Background(), opts); err != nil {
		t.Fatalf("installSkill: %v", err)
	}
	after, _ := os.ReadFile(paths.SkillFile)
	if !bytes.Equal(original, after) {
		t.Error("skill file changed when marker was absent; want skip")
	}
}
