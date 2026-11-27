package podman

import (
	"context"
	"io"
	"os"
	"os/exec"
)

type execRunner struct {
	exec    func(ctx context.Context, stdin io.Reader, args ...string) ([]byte, error)
	ttyExec func(ctx context.Context, args []string) error
}

func (r execRunner) Exec(ctx context.Context, stdin io.Reader, args []string) ([]byte, error) {
	var all []string
	if stdin != nil {
		all = append([]string{"exec", "-i"}, args...)
	} else {
		all = append([]string{"exec"}, args...)
	}
	return r.exec(ctx, stdin, all...)
}

func (r execRunner) ExecTTY(ctx context.Context, args []string) error {
	all := append([]string{"exec", "-i", "-t"}, args...)
	return r.ttyExec(ctx, all)
}

func (r Runner) Exec(ctx context.Context, stdin io.Reader, args ...string) ([]byte, error) {
	er := execRunner{exec: r.Run}
	return er.Exec(ctx, stdin, args)
}

func (r Runner) ExecTTY(ctx context.Context, args ...string) error {
	er := execRunner{ttyExec: func(ctx context.Context, all []string) error {
		cmd := exec.CommandContext(ctx, r.Path, all...)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return runError(err)
		}
		return nil
	}}
	return er.ExecTTY(ctx, args)
}
