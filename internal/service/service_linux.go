//go:build linux

package service

import (
	"context"
	"runtime"

	"github.com/kanthorlabs/sag/internal/stepline"
)

func Start(ctx context.Context, cfg Config, out stepline.Writer) error {
	return unsupportedPlatformErr(runtime.GOOS, runtime.GOARCH)
}

func Stop(ctx context.Context, cfg Config, out stepline.Writer) error {
	return unsupportedPlatformErr(runtime.GOOS, runtime.GOARCH)
}

func Running(ctx context.Context, cfg Config) (bool, error) {
	return false, unsupportedPlatformErr(runtime.GOOS, runtime.GOARCH)
}

func ApplyAutostart(ctx context.Context, cfg Config, enable bool, out stepline.Writer) (changed bool, err error) {
	return false, unsupportedPlatformErr(runtime.GOOS, runtime.GOARCH)
}
