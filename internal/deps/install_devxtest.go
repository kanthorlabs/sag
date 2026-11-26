//go:build devxtest

package deps

import (
	"context"
	"os"
	"os/exec"
)

var DefaultInstall = func(ctx context.Context, pkgPath string, _ bool) error {
	cmd := exec.CommandContext(ctx, "fakeinstall", pkgPath)
	cmd.Env = os.Environ()
	return cmd.Run()
}
