package dxh

import (
	"context"
	"os/exec"
	"testing"
)

type Runner interface {
	Output(ctx context.Context, name string, args ...string) ([]byte, error)
}

type osRunner struct {
	groupDir string
	binDir   string
}

func NewRunner() Runner { return osRunner{} }

func NewGroupRunner(groupDir, binDir string) Runner {
	return osRunner{groupDir: groupDir, binDir: binDir}
}

func (r osRunner) Output(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	if r.groupDir != "" {
		env, err := childEnv(r.groupDir, r.binDir)
		if err != nil {
			return nil, err
		}
		cmd.Env = env
	}
	return cmd.Output()
}

func NewProductRunner(t *testing.T) Runner {
	t.Helper()
	return NewGroupRunner(t.TempDir(), t.TempDir())
}
