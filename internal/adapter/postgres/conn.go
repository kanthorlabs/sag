package postgres

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

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

const (
	acquireConnectFailed = "connect failed"
	acquireTLSFailed     = "TLS handshake failed"
	acquireTimedOut      = "timed out waiting for a connection"
)

var errPoolSize = errors.New("pool_size is out of range")

type pool struct {
	p *pgxpool.Pool
}

func newPool(ctx context.Context, cc *pgx.ConnConfig, cfg core.ResourceConfig) (sql.Pool, error) {
	if cfg.PoolSize < 1 || cfg.PoolSize > math.MaxInt32 {
		return nil, errPoolSize
	}
	pc, err := pgxpool.ParseConfig("")
	if err != nil {
		return nil, fmt.Errorf("base pool config: %w", err)
	}
	pc.ConnConfig = cc.Copy()
	pc.MaxConns = int32(cfg.PoolSize)
	pc.MinConns = 0
	pc.MinIdleConns = 0
	pc.AfterConnect = runSessionGuard
	p, err := pgxpool.NewWithConfig(ctx, pc)
	if err != nil {
		return nil, fmt.Errorf("pool: %w", err)
	}
	return &pool{p: p}, nil
}

func runSessionGuard(ctx context.Context, c *pgx.Conn) error {
	for _, s := range sessionGuard() {
		if err := c.PgConn().ExecParams(ctx, s, nil, nil, nil, nil).Read().Err; err != nil {
			return fmt.Errorf("session guard: %w", err)
		}
	}
	return nil
}

func (p *pool) Acquire(ctx context.Context) (sql.Conn, error) {
	c, err := p.p.Acquire(ctx)
	if err != nil {
		return nil, acquireError(ctx, err)
	}
	return &conn{c: c}, nil
}

func (p *pool) Close() {
	p.p.Close()
}

func acquireError(ctx context.Context, err error) error {
	switch {
	case ctx.Err() != nil:
		return &core.Error{Code: core.CodeTimeout, Message: acquireTimedOut, Cause: err}
	case isTLSError(err):
		return &core.Error{Code: core.CodeResourceUnavailable, Message: acquireTLSFailed, Cause: err}
	default:
		return &core.Error{Code: core.CodeResourceUnavailable, Message: acquireConnectFailed, Cause: err}
	}
}

type conn struct {
	c *pgxpool.Conn
}

func (c *conn) Query(ctx context.Context, text string, args ...any) (sql.Rows, error) {
	r, err := c.c.Query(ctx, text, args...)
	if err != nil {
		return nil, engineError(ctx, err)
	}
	return &rows{ctx: ctx, r: r, types: c.c.Conn().TypeMap()}, nil
}

func (c *conn) Exec(ctx context.Context, text string) error {
	return engineError(ctx, c.c.Conn().PgConn().ExecParams(ctx, text, nil, nil, nil, nil).Read().Err)
}

func (c *conn) ExecParams(ctx context.Context, text string, args ...any) error {
	r, err := c.c.Query(ctx, text, args...)
	if err != nil {
		return engineError(ctx, err)
	}
	r.Close()
	return engineError(ctx, r.Err())
}

func (c *conn) Release() {
	if c.c == nil {
		return
	}
	c.c.Release()
	c.c = nil
}

func (c *conn) Close() error {
	if c.c == nil {
		return nil
	}
	pc := c.c.Hijack()
	c.c = nil
	ctx, cancel := context.WithTimeout(context.Background(), closeTimeout)
	defer cancel()
	return pc.Close(ctx)
}

type rows struct {
	ctx   context.Context
	r     pgx.Rows
	types *pgtype.Map
}

func (r *rows) Columns() []core.Column {
	fds := r.r.FieldDescriptions()
	columns := make([]core.Column, len(fds))
	for i, fd := range fds {
		columns[i] = core.Column{Name: fd.Name, Type: typeName(r.types, fd.DataTypeOID)}
	}
	return columns
}

func (r *rows) Next() bool {
	return r.r.Next()
}

func (r *rows) Values() ([]any, error) {
	fds := r.r.FieldDescriptions()
	raw := r.r.RawValues()
	values := make([]any, len(raw))
	for i := range raw {
		v, err := decodeValue(r.types, fds[i].DataTypeOID, raw[i])
		if err != nil {
			return nil, fmt.Errorf("column %d: %w", i+1, err)
		}
		values[i] = v
	}
	return values, nil
}

func (r *rows) Err() error {
	return engineError(r.ctx, r.r.Err())
}

func (r *rows) Close() {
	r.r.Close()
}

func typeName(m *pgtype.Map, oid uint32) string {
	if t, ok := m.TypeForOID(oid); ok {
		return t.Name
	}
	return "oid:" + strconv.FormatUint(uint64(oid), 10)
}

func decodeValue(m *pgtype.Map, oid uint32, raw []byte) (any, error) {
	if raw == nil {
		return nil, nil
	}
	switch oid {
	case pgtype.BoolOID:
		var v bool
		err := m.Scan(oid, pgtype.TextFormatCode, raw, &v)
		return v, err
	case pgtype.Int2OID, pgtype.Int4OID, pgtype.Int8OID:
		return strconv.ParseInt(string(raw), 10, 64)
	case pgtype.Float4OID, pgtype.Float8OID:
		return strconv.ParseFloat(string(raw), 64)
	case pgtype.ByteaOID:
		var v []byte
		err := m.Scan(oid, pgtype.TextFormatCode, raw, &v)
		return v, err
	case pgtype.DateOID, pgtype.TimestampOID, pgtype.TimestamptzOID:
		var v time.Time
		if err := m.Scan(oid, pgtype.TextFormatCode, raw, &v); err != nil {
			return string(raw), nil
		}
		return v, nil
	default:
		return string(raw), nil
	}
}

func engineError(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	switch {
	case errors.As(err, &pgErr):
		return &sql.EngineError{SQLState: pgErr.Code, Class: stateClass(pgErr.Code)}
	case ctx.Err() != nil || errors.Is(err, context.DeadlineExceeded):
		return &sql.EngineError{Class: sql.Timeout}
	default:
		return &sql.EngineError{Class: sql.Connect}
	}
}

func stateClass(state string) sql.Class {
	switch {
	case state == sqlStateReadOnly:
		return sql.ReadOnlyViolation
	case state == "57014" || state == "55P03":
		return sql.Timeout
	case strings.HasPrefix(state, "08") || state == "57P01" || state == "57P02" || state == "57P03":
		return sql.Connect
	default:
		return sql.Other
	}
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
