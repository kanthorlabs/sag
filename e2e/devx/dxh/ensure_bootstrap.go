package dxh

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var (
	ensureBootstrapCheckFn = defaultBootstrapCheck
	ensureBootstrapRunFn   = defaultBootstrapRun
)

func EnsureBootstrapped(t *testing.T) {
	t.Helper()
	r := NewRunner()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	home := os.Getenv("HOME")
	cfgDir := CfgDir(home)

	if ensureBootstrapCheckFn(ctx, r, cfgDir) {
		return
	}

	if err := ensureBootstrapRunFn(ctx, r); err != nil {
		t.Fatalf("EnsureBootstrapped: %v", err)
	}
}

func CfgDir(home string) string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "sag")
	}
	return filepath.Join(home, ".config", "sag")
}

func defaultBootstrapCheck(ctx context.Context, r Runner, cfgDir string) bool {
	if _, err := os.Stat(cfgDir); err != nil {
		return false
	}
	out, err := r.Output(ctx, "podman", "container", "inspect", "sag", "--format", "{{.State.Running}}")
	return err == nil && strings.TrimSpace(string(out)) == "true"
}

func defaultBootstrapRun(ctx context.Context, r Runner) error {
	sagBin, err := findSagBin()
	if err != nil {
		return err
	}
	out, err := r.Output(ctx, sagBin, "bootstrap",
		"--autostart", "yes",
		"--agent-setup", "claude-code=no",
		"--agent-setup", "pi=no")
	if err != nil {
		return fmt.Errorf("bootstrap: %w\n%s", err, out)
	}
	return nil
}

func findSagBin() (string, error) {
	if bin := os.Getenv("SAG_TEST_BIN"); bin != "" {
		return bin, nil
	}
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", fmt.Errorf("git rev-parse --show-toplevel: %w", err)
	}
	root := string(bytes.TrimSpace(out))
	p := filepath.Join(root, "bin", "sag-devxtest")
	if _, err := os.Stat(p); err != nil {
		return "", fmt.Errorf("sag-devxtest not found at %s; build with make build-devxtest", p)
	}
	return p, nil
}
