package postgres

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/kanthorlabs/sag/internal/core"
	"github.com/kanthorlabs/sag/internal/sql"
)

const closeTimeout = 5 * time.Second

const (
	preflightConnectFailed = "preflight connect failed"
	preflightTLSFailed     = "preflight TLS handshake failed"
	preflightQueryFailed   = "preflight query failed"
	preflightTimedOut      = "preflight timed out"
)

func newPool(ctx context.Context, cc *pgx.ConnConfig, cfg core.ResourceConfig) (sql.Pool, error) {
	return nil, errors.ErrUnsupported
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
