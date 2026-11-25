package testenv

import (
	"context"
	"errors"
	"io"
)

type Result struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

func Podman(ctx context.Context, stdin io.Reader, args ...string) (Result, error) {
	return Result{}, errors.ErrUnsupported
}
