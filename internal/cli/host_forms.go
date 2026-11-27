package cli

import (
	"context"
	"errors"
	"fmt"
	"os/exec"

	"github.com/kanthorlabs/sag/internal/hoststate"
	"github.com/kanthorlabs/sag/internal/podman"
)

var loadHostPodmanPath = func() (string, error) {
	p, err := hoststate.ResolvePaths()
	if err != nil {
		return "", err
	}
	answers, err := hoststate.LoadAnswers(p)
	if err != nil {
		return "", err
	}
	return hoststate.FindPodman(answers.PodmanPath)
}

var execHostTTY = func(ctx context.Context, podmanPath string, args []string) error {
	return podman.Runner{Path: podmanPath}.ExecTTY(ctx, args...)
}

var execHostForward = func(ctx context.Context, podmanPath string, std IO, args []string) int {
	cmd := exec.CommandContext(ctx, podmanPath, args...)
	cmd.Stdout = std.Out
	cmd.Stderr = std.Err
	if err := cmd.Run(); err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			code := exit.ExitCode()
			if code >= 0 && code <= 3 {
				return code
			}
		}
		return ExitFailure
	}
	return ExitOK
}

func ttyExit(err error) int {
	if err == nil {
		return ExitOK
	}
	var re *podman.RunError
	if errors.As(err, &re) {
		if re.ExitCode >= 0 && re.ExitCode <= 3 {
			return re.ExitCode
		}
	}
	return ExitFailure
}

func hostJWTGenerate(args []string, std IO) int {
	for _, a := range args {
		if a == "--json" {
			fmt.Fprintln(std.Err, jwtGenerateUsage)
			return ExitUsage
		}
	}
	if !std.IsTerminal(std.In) || !std.IsTerminal(std.Out) {
		fmt.Fprintln(std.Err, "sag: standard input and standard output must be terminals")
		return ExitRefusal
	}
	podmanPath, err := loadHostPodmanPath()
	if err != nil {
		return fail(std, err)
	}
	return ttyExit(execHostTTY(context.Background(), podmanPath, append([]string{"sag", "sag", "jwt", "generate"}, args...)))
}

func hostExtensionApprove(args []string, std IO) int {
	if !std.IsTerminal(std.In) || !std.IsTerminal(std.Out) {
		return fail(std, errApproveNoTerminal)
	}
	podmanPath, err := loadHostPodmanPath()
	if err != nil {
		return fail(std, err)
	}
	return ttyExit(execHostTTY(context.Background(), podmanPath, append([]string{"sag", "sag", "extension", "approve"}, args...)))
}

func hostExtensionRevoke(args []string, std IO) int {
	podmanPath, err := loadHostPodmanPath()
	if err != nil {
		return fail(std, err)
	}
	return execHostForward(context.Background(), podmanPath, std, append([]string{"exec", "sag", "sag", "extension", "revoke"}, args...))
}

func hostResourceTest(args []string, std IO) int {
	podmanPath, err := loadHostPodmanPath()
	if err != nil {
		return fail(std, err)
	}
	return execHostForward(context.Background(), podmanPath, std, append([]string{"exec", "sag", "sag", "resource", "test"}, args...))
}
