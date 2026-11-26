//go:build linux

package service

import (
	"context"
	"fmt"
	"runtime"
)

func Start(ctx context.Context, cfg Config) error {
	return fmt.Errorf("unsupported platform %s/%s", runtime.GOOS, runtime.GOARCH)
}

func Stop(ctx context.Context, cfg Config) error {
	return fmt.Errorf("unsupported platform %s/%s", runtime.GOOS, runtime.GOARCH)
}
