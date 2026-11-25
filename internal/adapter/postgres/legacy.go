package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/kanthorlabs/sag/internal/core"
)

type resource struct {
	cfg        core.ResourceConfig
	connConfig *pgx.ConnConfig
}

func (r *resource) Execute(ctx context.Context, capability, operation string, input json.RawMessage, lim core.Limits) (core.Result, error) {
	return core.Result{}, errors.ErrUnsupported
}

func (r *resource) Close() error {
	return nil
}

func (r *resource) Preflight(ctx context.Context) (core.PreflightReport, error) {
	conn, err := pgx.ConnectConfig(ctx, r.connConfig.Copy())
	if err != nil {
		return core.PreflightReport{}, preflightError(ctx, preflightConnectFailed, fmt.Errorf("connect: %w", err))
	}
	defer closeConn(ctx, conn)
	version, err := readVersion(ctx, conn)
	if err != nil {
		return core.PreflightReport{}, preflightError(ctx, preflightQueryFailed, err)
	}
	engine := core.PreflightCheck{Name: checkEngine, Passed: strings.HasPrefix(version, enginePrefix), Detail: version}
	return core.PreflightReport{Passed: engine.Passed, Checks: []core.PreflightCheck{engine}}, nil
}

func closeConn(ctx context.Context, conn *pgx.Conn) {
	closeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), closeTimeout)
	defer cancel()
	conn.Close(closeCtx)
}

func execText(ctx context.Context, conn *pgx.Conn, sql string) error {
	return conn.PgConn().ExecParams(ctx, sql, nil, nil, nil, nil).Read().Err
}

func readVersion(ctx context.Context, conn *pgx.Conn) (string, error) {
	if err := execText(ctx, conn, "BEGIN TRANSACTION READ ONLY"); err != nil {
		return "", fmt.Errorf("begin: %w", err)
	}
	var version string
	queryErr := conn.QueryRow(ctx, "SELECT version()").Scan(&version)
	if err := execText(ctx, conn, "ROLLBACK"); err != nil {
		return "", errors.Join(queryErr, fmt.Errorf("rollback: %w", err))
	}
	if queryErr != nil {
		return "", fmt.Errorf("select version: %w", queryErr)
	}
	return version, nil
}
