package dxh

import (
	"context"
	"os/exec"
)

type Runner interface {
	Output(ctx context.Context, name string, args ...string) ([]byte, error)
}

type osRunner struct{}

func NewRunner() Runner { return osRunner{} }

func (osRunner) Output(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).Output()
}
