package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os/exec"
	"time"

	"github.com/kanthorlabs/sag/internal/podman"
)

var runPodmanStream = func(ctx context.Context, out, errOut io.Writer, path string, args []string) error {
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.Stdout = out
	cmd.Stderr = errOut
	return cmd.Run()
}

var utcToday = func() string {
	return time.Now().UTC().Format("2006-01-02")
}

func logsDaemon(args []string, std IO) int {
	fs := flag.NewFlagSet("logs daemon", flag.ContinueOnError)
	fs.SetOutput(std.Err)
	follow := fs.Bool("follow", false, "")
	since := fs.String("since", "", "")
	tail := fs.String("tail", "", "")
	if err := fs.Parse(args); err != nil {
		return ExitUsage
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(std.Err, "sag: logs daemon: unexpected arguments")
		return ExitUsage
	}

	cfg, release, err := buildCfgNoLockFn()
	if err != nil {
		return fail(std, err)
	}
	defer release()

	runner, ok := cfg.Podman.(podman.Runner)
	if !ok {
		return fail(std, errNotImplemented)
	}

	logsArgs := []string{"logs"}
	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "follow":
			if *follow {
				logsArgs = append(logsArgs, "--follow")
			}
		case "since":
			logsArgs = append(logsArgs, "--since", *since)
		case "tail":
			logsArgs = append(logsArgs, "--tail", *tail)
		}
	})
	logsArgs = append(logsArgs, "sag")

	if err := runPodmanStream(context.Background(), std.Out, std.Err, runner.Path, logsArgs); err != nil {
		return fail(std, err)
	}
	return ExitOK
}

func logsAudit(args []string, std IO) int {
	defaultDate := utcToday()
	fs := flag.NewFlagSet("logs audit", flag.ContinueOnError)
	fs.SetOutput(std.Err)
	date := fs.String("date", defaultDate, "")
	if err := fs.Parse(args); err != nil {
		return ExitUsage
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(std.Err, "sag: logs audit: unexpected arguments")
		return ExitUsage
	}

	if _, err := time.Parse(auditDateFormat, *date); err != nil {
		fmt.Fprintf(std.Err, "sag: logs audit: invalid date %q: use YYYY-MM-DD\n", *date)
		return ExitUsage
	}

	cfg, release, err := buildCfgNoLockFn()
	if err != nil {
		return fail(std, err)
	}
	defer release()

	runner, ok := cfg.Podman.(podman.Runner)
	if !ok {
		return fail(std, errNotImplemented)
	}

	execArgs := []string{"exec", "sag", "sag", "audit", "read", "--date", *date}

	if err := runPodmanStream(context.Background(), std.Out, std.Err, runner.Path, execArgs); err != nil {
		return fail(std, err)
	}
	return ExitOK
}
