//go:build !devxtest

package deps

import (
	"context"
	"os/exec"
)

var DefaultInstall = func(ctx context.Context, pkgPath string, noInteractive bool) error {
	return runInstall(ctx, pkgPath, noInteractive, func(ctx context.Context, args []string) error {
		return exec.CommandContext(ctx, args[0], args[1:]...).Run()
	})
}
