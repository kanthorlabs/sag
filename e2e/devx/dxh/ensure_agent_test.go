package dxh

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureAgentSetupSkipsWhenPresent(t *testing.T) {
	claudeDir := t.TempDir()
	piDir := t.TempDir()

	mcpContent := []byte(`{"mcpServers":{"sag":{"type":"http","url":"https://localhost:8443/mcp"}}}`)
	if err := os.WriteFile(filepath.Join(claudeDir, ".claude.json"), mcpContent, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(piDir, "mcp.json"), mcpContent, 0o600); err != nil {
		t.Fatal(err)
	}

	prevClaude := ActiveAgentClaudeDir
	prevPi := ActiveAgentPiDir
	t.Cleanup(func() {
		ActiveAgentClaudeDir = prevClaude
		ActiveAgentPiDir = prevPi
	})
	ActiveAgentClaudeDir = claudeDir
	ActiveAgentPiDir = piDir

	runCalled := false
	oldRun := ensureAgentRunFn
	t.Cleanup(func() { ensureAgentRunFn = oldRun })
	ensureAgentRunFn = func(t *testing.T) error {
		runCalled = true
		return nil
	}

	oldBootstrap := ensureAgentBootstrapFn
	t.Cleanup(func() { ensureAgentBootstrapFn = oldBootstrap })
	ensureAgentBootstrapFn = func(t *testing.T) {}

	EnsureAgentSetup(t)

	if runCalled {
		t.Fatal("agent run was called even though setup already present")
	}
}

func TestEnsureAgentSetupRunsWhenAbsent(t *testing.T) {
	prevClaude := ActiveAgentClaudeDir
	prevPi := ActiveAgentPiDir
	t.Cleanup(func() {
		ActiveAgentClaudeDir = prevClaude
		ActiveAgentPiDir = prevPi
	})
	ActiveAgentClaudeDir = ""
	ActiveAgentPiDir = ""

	runCalled := false
	oldRun := ensureAgentRunFn
	t.Cleanup(func() { ensureAgentRunFn = oldRun })
	ensureAgentRunFn = func(t *testing.T) error {
		runCalled = true
		return nil
	}

	oldBootstrap := ensureAgentBootstrapFn
	t.Cleanup(func() { ensureAgentBootstrapFn = oldBootstrap })
	ensureAgentBootstrapFn = func(t *testing.T) {}

	EnsureAgentSetup(t)

	if !runCalled {
		t.Fatal("agent run was not called even though setup was absent")
	}
}
