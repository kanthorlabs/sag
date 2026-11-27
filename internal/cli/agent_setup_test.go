package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kanthorlabs/sag/internal/agentsetup"
)

func TestAgentSetupUnknownHarness(t *testing.T) {
	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}
	std := IO{
		In:         strings.NewReader(""),
		Out:        out,
		Err:        errOut,
		IsTerminal: func(any) bool { return false },
		Mode:       ModeHost,
	}
	code := agentSetup([]string{"unknown-harness"}, std)
	if code != ExitUsage {
		t.Errorf("exit = %d; want ExitUsage (%d)", code, ExitUsage)
	}
}

func TestAgentSetupMissingArg(t *testing.T) {
	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}
	std := IO{
		In:         strings.NewReader(""),
		Out:        out,
		Err:        errOut,
		IsTerminal: func(any) bool { return false },
		Mode:       ModeHost,
	}
	code := agentSetup([]string{}, std)
	if code != ExitUsage {
		t.Errorf("exit = %d; want ExitUsage (%d)", code, ExitUsage)
	}
}

func TestAgentSetupBadDenyRules(t *testing.T) {
	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}
	std := IO{
		In:         strings.NewReader(""),
		Out:        out,
		Err:        errOut,
		IsTerminal: func(any) bool { return false },
		Mode:       ModeHost,
	}
	code := agentSetup([]string{"claude-code", "--deny-rules", "maybe"}, std)
	if code != ExitUsage {
		t.Errorf("exit = %d; want ExitUsage (%d)", code, ExitUsage)
	}
}

func TestParseAgentSetupFlags(t *testing.T) {
	std := IO{
		In:         strings.NewReader(""),
		Out:        &bytes.Buffer{},
		Err:        &bytes.Buffer{},
		IsTerminal: func(any) bool { return false },
		Mode:       ModeHost,
	}
	opts, ok := parseAgentSetupFlags([]string{"claude-code", "--sub", "myagent", "--deny-rules", "yes"}, std)
	if !ok {
		t.Fatal("parseAgentSetupFlags returned false")
	}
	if string(opts.Harness) != "claude-code" {
		t.Errorf("Harness = %q; want claude-code", opts.Harness)
	}
	if opts.Sub != "myagent" {
		t.Errorf("Sub = %q; want myagent", opts.Sub)
	}
	if opts.DenyRules != "yes" {
		t.Errorf("DenyRules = %q; want yes", opts.DenyRules)
	}
}

func TestParseAgentSetupDefaultSub(t *testing.T) {
	std := IO{
		In:         strings.NewReader(""),
		Out:        &bytes.Buffer{},
		Err:        &bytes.Buffer{},
		IsTerminal: func(any) bool { return false },
		Mode:       ModeHost,
	}
	opts, ok := parseAgentSetupFlags([]string{"pi"}, std)
	if !ok {
		t.Fatal("parseAgentSetupFlags returned false")
	}
	if opts.Sub != "pi" {
		t.Errorf("Sub = %q; want pi (default equals harness name)", opts.Sub)
	}
}

func TestParseAgentSetupBadSubExit2(t *testing.T) {
	std := IO{
		In:         strings.NewReader(""),
		Out:        &bytes.Buffer{},
		Err:        &bytes.Buffer{},
		IsTerminal: func(any) bool { return false },
		Mode:       ModeHost,
	}
	_, ok := parseAgentSetupFlags([]string{"claude-code", "--sub", "Bad"}, std)
	if ok {
		t.Error("parseAgentSetupFlags returned true for invalid sub 'Bad'; want false")
	}
}

func TestParseAgentSetupHarnessFirstFlagsAfter(t *testing.T) {
	std := IO{
		In:         strings.NewReader(""),
		Out:        &bytes.Buffer{},
		Err:        &bytes.Buffer{},
		IsTerminal: func(any) bool { return false },
		Mode:       ModeHost,
	}
	opts, ok := parseAgentSetupFlags([]string{"claude-code", "--deny-rules", "no"}, std)
	if !ok {
		t.Fatal("parseAgentSetupFlags returned false for harness-first form")
	}
	if opts.DenyRules != "no" {
		t.Errorf("DenyRules = %q; want no", opts.DenyRules)
	}
}

func newTestIO() IO {
	return IO{
		In:         strings.NewReader(""),
		Out:        &bytes.Buffer{},
		Err:        &bytes.Buffer{},
		IsTerminal: func(any) bool { return false },
		Mode:       ModeHost,
	}
}

func TestB102ContainerModeExit1(t *testing.T) {
	std := newTestIO()
	std.Mode = ModeContainer
	code := agentSetup([]string{"claude-code"}, std)
	if code != ExitFailure {
		t.Errorf("B102: exit = %d; want ExitFailure (%d)", code, ExitFailure)
	}
}

func TestB91PiDenyRulesExit2(t *testing.T) {
	std := newTestIO()
	code := agentSetup([]string{"pi", "--deny-rules", "yes"}, std)
	if code != ExitUsage {
		t.Errorf("B91: pi --deny-rules exit = %d; want ExitUsage (%d)", code, ExitUsage)
	}
}

func TestB91ClaudeCodeGuardExit2(t *testing.T) {
	std := newTestIO()
	code := agentSetup([]string{"claude-code", "--guard", "yes"}, std)
	if code != ExitUsage {
		t.Errorf("B91: claude-code --guard exit = %d; want ExitUsage (%d)", code, ExitUsage)
	}
}

func TestB91ClaudeCodeShellProfileExit2(t *testing.T) {
	std := newTestIO()
	code := agentSetup([]string{"claude-code", "--shell-profile", "yes"}, std)
	if code != ExitUsage {
		t.Errorf("B91: claude-code --shell-profile exit = %d; want ExitUsage (%d)", code, ExitUsage)
	}
}

func TestB85RemoteModeNoPodmanInPath(t *testing.T) {
	std := newTestIO()
	errBuf := std.Err.(*bytes.Buffer)

	t.Setenv("PATH", t.TempDir())

	_, ok := parseAgentSetupFlags([]string{"claude-code", "--url", "https://remote.example:8443/mcp"}, std)
	if !ok {
		t.Fatal("B85: parseAgentSetupFlags returned false for remote URL")
	}
	if strings.Contains(errBuf.String(), "podman") {
		t.Errorf("B85: error message mentions podman for remote mode: %q", errBuf.String())
	}
}

func TestS64RelativeConfigDirMadeAbsolute(t *testing.T) {
	dir := t.TempDir()
	rel := filepath.Base(dir)
	origDir, err := os.Getwd()
	if err != nil {
		t.Skip("cannot get cwd:", err)
	}
	if err := os.Chdir(filepath.Dir(dir)); err != nil {
		t.Skip("cannot chdir:", err)
	}
	t.Cleanup(func() { _ = os.Chdir(origDir) })

	env := map[string]string{
		"CLAUDE_CONFIG_DIR": rel,
	}
	out := makeAbsoluteConfigDirs(env)
	if !filepath.IsAbs(out["CLAUDE_CONFIG_DIR"]) {
		t.Errorf("S64: CLAUDE_CONFIG_DIR = %q; want absolute path", out["CLAUDE_CONFIG_DIR"])
	}
}

func TestS65ConflictNoDuplicateMessage(t *testing.T) {
	outBuf := &bytes.Buffer{}
	errBuf := &bytes.Buffer{}
	std := IO{
		In:         strings.NewReader(""),
		Out:        outBuf,
		Err:        errBuf,
		IsTerminal: func(any) bool { return false },
		Mode:       ModeHost,
	}

	t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir())
	t.Setenv("PATH", t.TempDir())

	code := agentSetup([]string{"claude-code"}, std)
	if code != ExitFailure {
		t.Logf("S65: exit = %d (want ExitFailure for conflict)", code)
	}

	combined := outBuf.String() + errBuf.String()
	failCount := strings.Count(combined, "fail ")
	planConflictsCount := strings.Count(combined, "plan conflicts")
	if planConflictsCount > 0 {
		t.Errorf("S65: 'plan conflicts' appeared %d times; want 0 (conflicts should not double-print)", planConflictsCount)
	}
	_ = failCount
}

func TestAgentSetupNoTerminalForJWTExit3(t *testing.T) {
	std := newTestIO()
	code := agentSetupExit(agentsetup.ErrNoTerminalForJWT, std)
	if code != ExitRefusal {
		t.Errorf("exit = %d; want ExitRefusal (%d)", code, ExitRefusal)
	}
	if !strings.Contains(std.Err.(*bytes.Buffer).String(), "no terminal to read the JWT") {
		t.Errorf("stderr = %q; want the JWT terminal message", std.Err.(*bytes.Buffer).String())
	}
}
