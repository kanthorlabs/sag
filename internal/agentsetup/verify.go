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
	cmd.WaitDelay = verifyTimeout
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
	var sagLine string
	switch opts.Harness {
	case HarnessPi:
		connected, sagLine, err = verifyPi(verifyCtx, opts, env, tmpDir)
	default:
		connected, sagLine, err = verifyClaudeCode(verifyCtx, opts, env, tmpDir)
	}
	if err != nil {
		out.Line(stepline.StatusFail, "verify", string(opts.Harness)+" "+err.Error())
		return fmt.Errorf("verify %s: %w", opts.Harness, err)
	}
	if !connected {
		detail := string(opts.Harness)
		if sagLine != "" {
			detail += " " + sagLine
		}
		out.Line(stepline.StatusFail, "verify", detail)
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

func verifyClaudeCode(ctx context.Context, opts Options, env []string, dir string) (bool, string, error) {
	harnessCmd, err := lookupHarness(opts, "claude")
	if err != nil {
		return false, "", err
	}
	output, _ := runVerifyChild(ctx, harnessCmd, []string{"mcp", "list"}, env, dir)
	for _, line := range strings.Split(string(output), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "sag: ") {
			if strings.Contains(trimmed, "✔ Connected") {
				return true, trimmed, nil
			}
			return false, trimmed, nil
		}
	}
	return false, "", nil
}

func verifyPi(ctx context.Context, opts Options, env []string, dir string) (bool, string, error) {
	harnessCmd, err := lookupHarness(opts, "pi")
	if err != nil {
		return false, "", err
	}
	output, _ := runVerifyChild(ctx, harnessCmd, []string{"mcp", "list", "--json"}, env, dir)
	state := parsePiMCPState(output)
	if state == "connected" {
		return true, "", nil
	}
	return false, "pi state: " + state, nil
}

type piMCPListOutput struct {
	Servers []struct {
		Name  string `json:"name"`
		State string `json:"state"`
	} `json:"servers"`
}

func parsePiMCPState(data []byte) string {
	data = bytes.TrimSpace(data)
	if len(data) == 0 {
		return ""
	}
	var result piMCPListOutput
	if err := json.Unmarshal(data, &result); err != nil {
		return ""
	}
	for _, s := range result.Servers {
		if s.Name == "sag" {
			return s.State
		}
	}
	return ""
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
