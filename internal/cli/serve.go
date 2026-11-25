package cli

import (
	"context"
	"fmt"
	"os/signal"
	"syscall"

	"github.com/kanthorlabs/sag/internal/daemon"
)

const serveUsage = "usage: sag serve"

func serve(args []string, std IO) int {
	if len(args) != 0 {
		fmt.Fprintln(std.Err, serveUsage)
		return ExitUsage
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	err := daemon.Run(ctx, daemon.Options{
		ConfigPath: paths.Config,
		SecretsDir: paths.Secrets,
		SecretUID:  paths.SecretUID,
		StateDir:   paths.State,
		Version:    Version,
		Log:        std.Err,
	})
	if err != nil {
		return fail(std, err)
	}
	return ExitOK
}
