package deps

import "context"

func sudoCheckArgs(noInteractive bool) []string {
	if noInteractive {
		return []string{"sudo", "-n", "true"}
	}
	return []string{"sudo", "-v"}
}

func installerArgs(pkgPath string, noInteractive bool) []string {
	if noInteractive {
		return []string{"sudo", "-n", "installer", "-pkg", pkgPath, "-target", "/"}
	}
	return []string{"sudo", "installer", "-pkg", pkgPath, "-target", "/"}
}

func runInstall(ctx context.Context, pkgPath string, noInteractive bool,
	run func(context.Context, []string) error) error {
	if err := run(ctx, sudoCheckArgs(noInteractive)); err != nil {
		return &SudoRefusal{}
	}
	return run(ctx, installerArgs(pkgPath, noInteractive))
}
