//go:build !devxtest

package deps

import (
	"context"
	"os/exec"
)

var DefaultInstall = func(ctx context.Context, pkgPath string, noInteractive bool) error {
	if noInteractive {
		if err := exec.CommandContext(ctx, "sudo", "-n", "installer", "-pkg", pkgPath, "-target", "/").Run(); err != nil {
			return &SudoRefusal{}
		}
		return nil
	}
	return exec.CommandContext(ctx, "sudo", "installer", "-pkg", pkgPath, "-target", "/").Run()
}
