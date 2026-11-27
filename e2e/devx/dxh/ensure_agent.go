package dxh

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var (
	ActiveAgentClaudeDir   string
	ActiveAgentPiDir       string
	ActiveAgentZDOTDIR     string
	ActiveAgentBinDir      string
	ActiveAgentSetupOutput []string

	ensureAgentBootstrapFn = func(t *testing.T) { EnsureBootstrapped(t) }
	ensureAgentRunFn       = defaultAgentRun
)

func EnsureAgentSetup(t *testing.T) {
	t.Helper()
	ensureAgentBootstrapFn(t)

	if agentSetupPresent() {
		return
	}

	if err := ensureAgentRunFn(t); err != nil {
		t.Fatalf("EnsureAgentSetup: %v", err)
	}
}

func agentSetupPresent() bool {
	if ActiveAgentClaudeDir == "" || ActiveAgentPiDir == "" {
		return false
	}
	return hasMCPEntry(filepath.Join(ActiveAgentClaudeDir, ".claude.json")) &&
		hasMCPEntry(filepath.Join(ActiveAgentPiDir, "mcp.json"))
}

func hasMCPEntry(mcpFile string) bool {
	data, err := os.ReadFile(mcpFile)
	if err != nil {
		return false
	}
	var top struct {
		MCPServers map[string]json.RawMessage `json:"mcpServers"`
	}
	if err := json.Unmarshal(data, &top); err != nil {
		return false
	}
	_, ok := top.MCPServers["sag"]
	return ok
}

func defaultAgentRun(t *testing.T) error {
	t.Helper()

	bin, err := findSagBin()
	if err != nil {
		return err
	}
	root := filepath.Dir(filepath.Dir(bin))

	claudeDir, err := os.MkdirTemp("", "sag-agent-claude-*")
	if err != nil {
		return fmt.Errorf("create claude dir: %w", err)
	}
	piDir, err := os.MkdirTemp("", "sag-agent-pi-*")
	if err != nil {
		return fmt.Errorf("create pi dir: %w", err)
	}
	zdotdir, err := os.MkdirTemp("", "sag-agent-zdot-*")
	if err != nil {
		return fmt.Errorf("create zdotdir: %w", err)
	}
	binDir, err := os.MkdirTemp("", "sag-agent-bin-*")
	if err != nil {
		return fmt.Errorf("create binDir: %w", err)
	}

	if err := BuildFakeHarness(root, binDir); err != nil {
		return fmt.Errorf("build fakeharness: %w", err)
	}

	uid := fmt.Sprintf("%d", os.Getuid())
	r := NewRunner()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err := WaitLaunchAgentIdle(ctx, r, uid); err != nil {
		return fmt.Errorf("wait launch agent idle before agent setup: %w", err)
	}

	claudeOut, err := runAgentSetup(t, ctx, bin, binDir, claudeDir, piDir, "claude-code",
		"--deny-rules", "yes")
	if err != nil {
		return fmt.Errorf("agent setup claude-code: %w\n%s", err, claudeOut)
	}

	piOut, err := runAgentSetupPi(t, ctx, bin, binDir, claudeDir, piDir, zdotdir)
	if err != nil {
		return fmt.Errorf("agent setup pi: %w\n%s", err, piOut)
	}

	ActiveAgentClaudeDir = claudeDir
	ActiveAgentPiDir = piDir
	ActiveAgentZDOTDIR = zdotdir
	ActiveAgentBinDir = binDir
	ActiveAgentSetupOutput = []string{string(claudeOut), string(piOut)}
	return nil
}

func BuildFakeHarness(root, binDir string) error {
	out := filepath.Join(binDir, "fakeharness")
	cmd := exec.Command("go", "build", "-o", out, "./e2e/devx/cmd/fakeharness/")
	cmd.Dir = root
	cmd.Env = os.Environ()
	data, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("go build fakeharness: %w\n%s", err, data)
	}
	for _, name := range []string{"claude", "pi", "podman"} {
		link := filepath.Join(binDir, name)
		_ = os.Remove(link)
		if err := os.Symlink(out, link); err != nil {
			return fmt.Errorf("symlink %s: %w", name, err)
		}
	}
	return nil
}

func runAgentSetup(t *testing.T, ctx context.Context, bin, binDir, claudeDir, piDir, harness string, extraArgs ...string) ([]byte, error) {
	t.Helper()
	env, err := agentChildEnv(claudeDir, piDir, binDir)
	if err != nil {
		return nil, err
	}
	args := append([]string{"agent", "setup", harness}, extraArgs...)
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = env
	return cmd.CombinedOutput()
}

func runAgentSetupPi(t *testing.T, ctx context.Context, bin, binDir, claudeDir, piDir, zdotdir string) ([]byte, error) {
	t.Helper()
	env, err := agentChildEnv(claudeDir, piDir, binDir)
	if err != nil {
		return nil, err
	}
	env = append(env, "SHELL=/bin/zsh")
	env = append(env, "ZDOTDIR="+zdotdir)
	cmd := exec.CommandContext(ctx, bin, "agent", "setup", "pi",
		"--shell-profile", "yes",
		"--guard", "yes")
	cmd.Env = env
	return cmd.CombinedOutput()
}

func agentChildEnv(claudeDir, piDir, binDir string) ([]string, error) {
	var base []string
	for _, e := range os.Environ() {
		key, _, _ := strings.Cut(e, "=")
		switch key {
		case "HOME", "TMPDIR", "USER", "LANG":
			base = append(base, e)
		default:
			if strings.HasPrefix(key, "SAG_TEST_") {
				base = append(base, e)
			}
		}
	}
	base = append(base, "CLAUDE_CONFIG_DIR="+claudeDir)
	base = append(base, "PI_CODING_AGENT_DIR="+piDir)
	base = append(base, "PATH="+TestPATH(binDir))
	return base, nil
}

func ResetAgentSetup() {
	for _, dir := range []string{ActiveAgentClaudeDir, ActiveAgentPiDir, ActiveAgentZDOTDIR, ActiveAgentBinDir} {
		if dir != "" {
			os.RemoveAll(dir)
		}
	}
	ActiveAgentClaudeDir = ""
	ActiveAgentPiDir = ""
	ActiveAgentZDOTDIR = ""
	ActiveAgentBinDir = ""
	ActiveAgentSetupOutput = nil
}

func NewAgentGroupRunner(extraEnv ...string) Runner {
	return agentGroupRunner{extraEnv: extraEnv}
}

type agentGroupRunner struct {
	extraEnv []string
}

func (r agentGroupRunner) Output(ctx context.Context, name string, args ...string) ([]byte, error) {
	env, err := agentChildEnv(ActiveAgentClaudeDir, ActiveAgentPiDir, ActiveAgentBinDir)
	if err != nil {
		return nil, err
	}
	env = append(env, r.extraEnv...)
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = env
	return cmd.Output()
}
