package dxh

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

var (
	newRunnerFn          = func() Runner { return NewRunner() }
	ensureBootstrapRunFn = defaultBootstrapRun
)

func EnsureBootstrapped(t *testing.T) {
	t.Helper()
	r := newRunnerFn()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	home := os.Getenv("HOME")
	cfgDir := CfgDir(home)

	if defaultBootstrapCheck(ctx, r, cfgDir) {
		return
	}

	if err := ensureBootstrapRunFn(ctx, NewProductRunner(t)); err != nil {
		t.Fatalf("EnsureBootstrapped: %v", err)
	}
}

func CfgDir(home string) string {
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
	uid := strconv.Itoa(os.Getuid())
	return WaitLaunchAgentIdle(ctx, NewRunner(), uid)
}

func findSagBin() (string, error) {
	p, err := TestBinary()
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(p); err != nil {
		return "", fmt.Errorf("sag-devxtest not found at %s; build with make build-devxtest", p)
	}
	return p, nil
}
