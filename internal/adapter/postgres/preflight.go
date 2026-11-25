package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/kanthorlabs/sag/internal/core"
)

const (
	checkEngine  = "engine"
	enginePrefix = "PostgreSQL "
)

func (r *resource) Preflight(ctx context.Context) (core.PreflightReport, error) {
	conn, err := pgx.ConnectConfig(ctx, r.connConfig.Copy())
	if err != nil {
		return core.PreflightReport{}, fmt.Errorf("connect: %w", err)
	}
	defer conn.Close(context.WithoutCancel(ctx))
	version, err := readVersion(ctx, conn)
	if err != nil {
		return core.PreflightReport{}, err
	}
	engine := core.PreflightCheck{Name: checkEngine, Passed: strings.HasPrefix(version, enginePrefix), Detail: version}
	return core.PreflightReport{Passed: engine.Passed, Checks: []core.PreflightCheck{engine}}, nil
}

func readVersion(ctx context.Context, conn *pgx.Conn) (string, error) {
	if _, err := conn.Exec(ctx, "BEGIN TRANSACTION READ ONLY"); err != nil {
		return "", fmt.Errorf("begin: %w", err)
	}
	var version string
	queryErr := conn.QueryRow(ctx, "SELECT version()").Scan(&version)
	if _, err := conn.Exec(ctx, "ROLLBACK"); err != nil {
		return "", errors.Join(queryErr, fmt.Errorf("rollback: %w", err))
	}
	if queryErr != nil {
		return "", fmt.Errorf("select version: %w", queryErr)
	}
	return version, nil
}
