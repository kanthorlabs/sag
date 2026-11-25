package cli

import (
	"context"
	"fmt"
	"os/signal"
	"syscall"

	"github.com/kanthorlabs/sag/internal/daemon"
	"github.com/kanthorlabs/sag/internal/sql"
)

const serveUsage = "usage: sag serve"

var runDaemon = daemon.Run

func serveProduct(args []string, std IO) int {
	return serve(args, std, nil)
}

func serve(args []string, std IO, wrap func(sql.Dialect) sql.Dialect) int {
	if len(args) != 0 {
		fmt.Fprintln(std.Err, serveUsage)
		return ExitUsage
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	err := runDaemon(ctx, daemon.Options{
		ConfigPath:  paths.Config,
		SecretsDir:  paths.Secrets,
		SecretUID:   paths.SecretUID,
		StateDir:    paths.State,
		Version:     Version,
		Log:         std.Err,
		WrapDialect: wrap,
	})
	if err != nil {
		return fail(std, err)
	}
	return ExitOK
}
