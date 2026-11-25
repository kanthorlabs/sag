package harness

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/kanthorlabs/sag/internal/testenv"
)

const (
	claudeServerName = "sag"
	claudeModel      = "haiku"
	claudeTimeout    = 3 * time.Minute
	claudeAuthWait   = 30 * time.Second
)

type claudeMCPServer struct {
	Type    string            `json:"type"`
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers"`
}

type claudeMCPConfig struct {
	MCPServers map[string]claudeMCPServer `json:"mcpServers"`
}

func RunClaude(ctx context.Context, req ClaudeRequest) (testenv.Result, error) {
	bin, err := exec.LookPath("claude")
	if err != nil {
		return testenv.Result{}, fmt.Errorf("%w: claude is not in PATH", ErrClaudePrerequisite)
	}
	if err := claudeLoggedIn(ctx, bin); err != nil {
		return testenv.Result{}, err
	}

	dir, err := os.MkdirTemp("", "sag-claude-")
	if err != nil {
		return testenv.Result{}, err
	}
	defer os.RemoveAll(dir)

	cfg, err := json.Marshal(claudeMCPConfig{MCPServers: map[string]claudeMCPServer{
		claudeServerName: {
			Type:    "http",
			URL:     req.MCPURL,
			Headers: map[string]string{"Authorization": "Bearer " + req.Token},
		},
	}})
	if err != nil {
		return testenv.Result{}, err
	}
	configFile := filepath.Join(dir, "mcp.json")
	if err := os.WriteFile(configFile, cfg, 0o600); err != nil {
		return testenv.Result{}, err
	}
	caFile := filepath.Join(dir, "ca.pem")
	if err := os.WriteFile(caFile, req.CAPEM, 0o600); err != nil {
		return testenv.Result{}, err
	}

	runCtx, cancel := context.WithTimeout(ctx, claudeTimeout)
	defer cancel()
	cmd := exec.CommandContext(runCtx, bin,
		"-p", req.Prompt,
		"--model", claudeModel,
		"--mcp-config", configFile,
		"--strict-mcp-config",
		"--tools", "",
		"--allowedTools", strings.Join(req.AllowedTools, ","),
		"--no-session-persistence",
	)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "NODE_EXTRA_CA_CERTS="+caFile)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	res := testenv.Result{Stdout: stdout.String(), Stderr: stderr.String()}
	if runCtx.Err() != nil {
		return res, fmt.Errorf("harness: claude: %w", runCtx.Err())
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		res.ExitCode = exitErr.ExitCode()
		return res, nil
	}
	return res, err
}

func claudeLoggedIn(ctx context.Context, bin string) error {
	authCtx, cancel := context.WithTimeout(ctx, claudeAuthWait)
	defer cancel()
	out, _ := exec.CommandContext(authCtx, bin, "auth", "status", "--json").Output()
	var status struct {
		LoggedIn bool `json:"loggedIn"`
	}
	if json.Unmarshal(out, &status) != nil || !status.LoggedIn {
		return fmt.Errorf("%w: claude is not logged in", ErrClaudePrerequisite)
	}
	return nil
}
