package postgres

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/kanthorlabs/sag/internal/core"
)

const (
	checkEngine  = "engine"
	enginePrefix = "PostgreSQL "
	closeTimeout = 5 * time.Second
)

const (
	preflightConnectFailed = "preflight connect failed"
	preflightTLSFailed     = "preflight TLS handshake failed"
	preflightQueryFailed   = "preflight query failed"
	preflightTimedOut      = "preflight timed out"
)

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

func preflightError(ctx context.Context, message string, err error) error {
	switch {
	case ctx.Err() != nil || errors.Is(err, context.DeadlineExceeded):
		message = preflightTimedOut
	case isTLSError(err):
		message = preflightTLSFailed
	}
	return &core.Error{Code: core.CodeResourceUnavailable, Message: message, Cause: err}
}

func isTLSError(err error) bool {
	var verify *tls.CertificateVerificationError
	var authority x509.UnknownAuthorityError
	var hostname x509.HostnameError
	var invalid x509.CertificateInvalidError
	var alert tls.AlertError
	var record tls.RecordHeaderError
	return errors.As(err, &verify) || errors.As(err, &authority) || errors.As(err, &hostname) ||
		errors.As(err, &invalid) || errors.As(err, &alert) || errors.As(err, &record)
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
