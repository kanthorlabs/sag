//go:build linux

package deps

import (
	"context"
	"fmt"
	"os/exec"
)

func DefaultAptCandidateVersion(ctx context.Context, name string) (string, error) {
	out, err := exec.CommandContext(ctx, "apt-cache", "policy", name).Output()
	if err != nil {
		return "", fmt.Errorf("deps: apt-cache policy %s: %w", name, err)
	}
	return parseAptCachePolicy(string(out))
}

func aptInstallArgs(name string, noInteractive bool) []string {
	if noInteractive {
		return []string{"sudo", "-n", "apt-get", "install", "-y", name}
	}
	return []string{"sudo", "apt-get", "install", "-y", name}
}

func runAptInstall(ctx context.Context, name string, noInteractive bool, run func(context.Context, []string) error) error {
	if err := run(ctx, sudoCheckArgs(noInteractive)); err != nil {
		return &SudoRefusal{}
	}
	return run(ctx, aptInstallArgs(name, noInteractive))
}

func DefaultAptInstall(ctx context.Context, name string, noInteractive bool) error {
	return runAptInstall(ctx, name, noInteractive, func(ctx context.Context, args []string) error {
		return exec.CommandContext(ctx, args[0], args[1:]...).Run()
	})
}

func DefaultFindNewuidmap() bool {
	_, err := exec.LookPath("newuidmap")
	return err == nil
}
