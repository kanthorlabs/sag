package agentsetup

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/kanthorlabs/sag/internal/stepline"
)

const verifyTimeout = 60 * time.Second

var runVerifyChild = func(ctx context.Context, name string, args []string, env []string, dir string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = env
	cmd.Dir = dir
	return cmd.Output()
}

func verify(ctx context.Context, opts Options) error {
	out := stepline.New(agentOut)
	ca := caFilePath(opts)

	verifyCtx, cancel := context.WithTimeout(ctx, verifyTimeout)
	defer cancel()

	tmpDir, err := os.MkdirTemp("", "sag-verify-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	env := buildVerifyEnv(opts, ca)

	var connected bool
	switch opts.Harness {
	case HarnessPi:
		connected, err = verifyPi(verifyCtx, opts, env, tmpDir)
	default:
		connected, err = verifyClaudeCode(verifyCtx, opts, env, tmpDir)
	}
	if err != nil {
		out.Line(stepline.StatusFail, "verify", string(opts.Harness))
		return fmt.Errorf("verify %s: %w", opts.Harness, err)
	}
	if !connected {
		out.Line(stepline.StatusFail, "verify", string(opts.Harness))
		return fmt.Errorf("verify %s: sag not connected", opts.Harness)
	}
	out.Line(stepline.StatusOK, "verify", string(opts.Harness))
	return nil
}

func buildVerifyEnv(opts Options, caPath string) []string {
	var env []string
	for k, v := range opts.Env {
		env = append(env, k+"="+v)
	}
	switch opts.Harness {
	case HarnessPi:
		env = append(env, "NODE_EXTRA_CA_CERTS="+caPath)
	default:
		filtered := make([]string, 0, len(env))
		for _, e := range env {
			k, _, _ := strings.Cut(e, "=")
			if k != "NODE_EXTRA_CA_CERTS" {
				filtered = append(filtered, e)
			}
		}
		env = filtered
	}
	return env
}

func verifyClaudeCode(ctx context.Context, opts Options, env []string, dir string) (bool, error) {
	harnessCmd, err := lookupHarness(opts, "claude")
	if err != nil {
		return false, err
	}
	out, _ := runVerifyChild(ctx, harnessCmd, []string{"mcp", "list"}, env, dir)
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "sag: ") {
			return true, nil
		}
	}
	return false, nil
}

func verifyPi(ctx context.Context, opts Options, env []string, dir string) (bool, error) {
	harnessCmd, err := lookupHarness(opts, "pi")
	if err != nil {
		return false, err
	}
	out, _ := runVerifyChild(ctx, harnessCmd, []string{"mcp", "list", "--json"}, env, dir)
	return parsePiMCPState(out), nil
}

type piMCPListOutput struct {
	Servers []struct {
		Name  string `json:"name"`
		State string `json:"state"`
	} `json:"servers"`
}

func parsePiMCPState(data []byte) bool {
	data = bytes.TrimSpace(data)
	if len(data) == 0 {
		return false
	}
	var result piMCPListOutput
	if err := json.Unmarshal(data, &result); err != nil {
		return false
	}
	for _, s := range result.Servers {
		if s.Name == "sag" {
			return s.State == "connected"
		}
	}
	return false
}

func lookupHarness(opts Options, name string) (string, error) {
	pathEnv := opts.Env["PATH"]
	if pathEnv == "" {
		pathEnv = os.Getenv("PATH")
	}
	for _, dir := range strings.Split(pathEnv, ":") {
		candidate := dir + "/" + name
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("%s not found in PATH", name)
}

func lastLine(out string) string {
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.TrimSpace(lines[i]) != "" {
			return strings.TrimSpace(lines[i])
		}
	}
	return ""
}
