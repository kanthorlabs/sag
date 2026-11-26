//go:build linux

package service

import (
	"context"
	"fmt"
	"runtime"

	"github.com/kanthorlabs/sag/internal/stepline"
)

func Start(ctx context.Context, cfg Config, out stepline.Writer) error {
	return fmt.Errorf("unsupported platform %s/%s", runtime.GOOS, runtime.GOARCH)
}

func Stop(ctx context.Context, cfg Config, out stepline.Writer) error {
	return fmt.Errorf("unsupported platform %s/%s", runtime.GOOS, runtime.GOARCH)
}

func Running(ctx context.Context, cfg Config) (bool, error) {
	return false, fmt.Errorf("unsupported platform %s/%s", runtime.GOOS, runtime.GOARCH)
}

func ApplyAutostart(ctx context.Context, cfg Config, enable bool, out stepline.Writer) (changed bool, err error) {
	return false, fmt.Errorf("unsupported platform %s/%s", runtime.GOOS, runtime.GOARCH)
}
