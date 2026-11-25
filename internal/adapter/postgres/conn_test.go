package postgres

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/kanthorlabs/sag/internal/core"
	"github.com/kanthorlabs/sag/internal/secret"
	"github.com/kanthorlabs/sag/internal/sql"
)

func TestEngineErrorClasses(t *testing.T) {
	live := context.Background()
	expired, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	cases := []struct {
		name string
		ctx  context.Context
		err  error
		want sql.EngineError
	}{
		{"read only", live, &pgconn.PgError{Code: "25006"}, sql.EngineError{SQLState: "25006", Class: sql.ReadOnlyViolation}},
		{"wrapped read only", live, fmt.Errorf("query: %w", &pgconn.PgError{Code: "25006"}), sql.EngineError{SQLState: "25006", Class: sql.ReadOnlyViolation}},
		{"statement timeout", live, &pgconn.PgError{Code: "57014"}, sql.EngineError{SQLState: "57014", Class: sql.Timeout}},
		{"lock timeout", live, &pgconn.PgError{Code: "55P03"}, sql.EngineError{SQLState: "55P03", Class: sql.Timeout}},
		{"syntax", live, &pgconn.PgError{Code: "42601"}, sql.EngineError{SQLState: "42601", Class: sql.Other}},
		{"connection failure", live, &pgconn.PgError{Code: "08006"}, sql.EngineError{SQLState: "08006", Class: sql.Connect}},
		{"admin shutdown", live, &pgconn.PgError{Code: "57P01"}, sql.EngineError{SQLState: "57P01", Class: sql.Connect}},
		{"deadline", live, fmt.Errorf("read: %w", context.DeadlineExceeded), sql.EngineError{Class: sql.Timeout}},
		{"expired context", expired, io.ErrUnexpectedEOF, sql.EngineError{Class: sql.Timeout}},
		{"lost connection", live, io.ErrUnexpectedEOF, sql.EngineError{Class: sql.Connect}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := engineError(c.ctx, c.err)
			var got *sql.EngineError
			if !errors.As(err, &got) || *got != c.want {
				t.Fatalf("engineError() = %v; want %+v", err, c.want)
			}
		})
	}
	if err := engineError(live, nil); err != nil {
		t.Fatalf("engineError(nil) = %v; want nil", err)
	}
}

func TestValueDecoding(t *testing.T) {
	m := pgtype.NewMap()
	stamp := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
	cases := []struct {
		name string
		oid  uint32
		raw  []byte
		want any
	}{
		{"null", pgtype.TextOID, nil, nil},
		{"bool", pgtype.BoolOID, []byte("t"), true},
		{"int2", pgtype.Int2OID, []byte("-7"), int64(-7)},
		{"int4", pgtype.Int4OID, []byte("42"), int64(42)},
		{"int8", pgtype.Int8OID, []byte("9007199254740993"), int64(9007199254740993)},
		{"float4", pgtype.Float4OID, []byte("0.1"), 0.1},
		{"float8", pgtype.Float8OID, []byte("1.5"), 1.5},
		{"text", pgtype.TextOID, []byte("abc"), "abc"},
		{"numeric", pgtype.NumericOID, []byte("1.250"), "1.250"},
		{"bytea", pgtype.ByteaOID, []byte(`\x0102`), []byte{1, 2}},
		{"timestamptz", pgtype.TimestamptzOID, []byte("2024-01-02 03:04:05+00"), stamp},
		{"timestamp", pgtype.TimestampOID, []byte("2024-01-02 03:04:05"), stamp},
		{"date", pgtype.DateOID, []byte("2024-01-02"), time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)},
		{"infinite timestamptz", pgtype.TimestamptzOID, []byte("infinity"), "infinity"},
		{"int array", pgtype.Int4ArrayOID, []byte("{1,2}"), "{1,2}"},
		{"json", pgtype.JSONBOID, []byte(`{"a": 1}`), `{"a": 1}`},
		{"unknown oid", 99999, []byte("x"), "x"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := decodeValue(m, c.oid, c.raw)
			if err != nil {
				t.Fatalf("decodeValue: %v", err)
			}
			if gotTime, ok := got.(time.Time); ok {
				if wantTime, ok := c.want.(time.Time); !ok || !gotTime.Equal(wantTime) {
					t.Fatalf("decodeValue() = %v; want %v", got, c.want)
				}
				return
			}
			if !reflect.DeepEqual(got, c.want) {
				t.Fatalf("decodeValue() = %#v; want %#v", got, c.want)
			}
		})
	}
}

func TestTypeName(t *testing.T) {
	m := pgtype.NewMap()
	if got := typeName(m, pgtype.Int4OID); got != "int4" {
		t.Fatalf("typeName(int4) = %q", got)
	}
	if got := typeName(m, 99999); got != "oid:99999" {
		t.Fatalf("typeName(99999) = %q", got)
	}
}

func unreachablePool(t *testing.T) sql.Pool {
	t.Helper()
	cfg := core.ResourceConfig{Name: "down", Kind: Kind, TLS: core.TLSDisable, PoolSize: 1, ConnectTimeout: 2 * time.Second}
	cc, err := connConfig(cfg, secret.NewCredential("conn-down", []byte("postgres://sag@127.0.0.1:1/app")))
	if err != nil {
		t.Fatal(err)
	}
	p, err := newPool(context.Background(), cc, cfg)
	if err != nil {
		t.Fatalf("newPool: %v; want no connect at construction", err)
	}
	t.Cleanup(p.Close)
	return p
}

func TestNewPoolDoesNotConnect(t *testing.T) {
	unreachablePool(t)
}

func TestNewPoolRejectsZeroSize(t *testing.T) {
	cfg := core.ResourceConfig{Name: "zero", Kind: Kind, TLS: core.TLSDisable, ConnectTimeout: time.Second}
	cc, err := connConfig(cfg, secret.NewCredential("conn-zero", []byte("postgres://sag@127.0.0.1:1/app")))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := newPool(context.Background(), cc, cfg); err == nil {
		t.Fatal("newPool with PoolSize 0 = nil error")
	}
}

func TestAcquireConnectFailureIsResourceUnavailable(t *testing.T) {
	p := unreachablePool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err := p.Acquire(ctx)
	var coreErr *core.Error
	if !errors.As(err, &coreErr) || coreErr.Code != core.CodeResourceUnavailable || coreErr.Message != core.PreflightConnectFailed {
		t.Fatalf("Acquire error = %v; want core.Error %s %q", err, core.CodeResourceUnavailable, core.PreflightConnectFailed)
	}
}

func TestAcquireErrorKeepsTLSClass(t *testing.T) {
	err := acquireError(context.Background(), fmt.Errorf("connect: %w", &tls.CertificateVerificationError{Err: x509.UnknownAuthorityError{}}))
	var coreErr *core.Error
	if !errors.As(err, &coreErr) || coreErr.Code != core.CodeResourceUnavailable || coreErr.Message != core.PreflightTLSFailed {
		t.Fatalf("acquireError = %v; want core.Error %s %q", err, core.CodeResourceUnavailable, core.PreflightTLSFailed)
	}
}

func TestAcquireEndedContextIsTimeout(t *testing.T) {
	p := unreachablePool(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := p.Acquire(ctx)
	var coreErr *core.Error
	if !errors.As(err, &coreErr) || coreErr.Code != core.CodeTimeout {
		t.Fatalf("Acquire error = %v; want core.Error %s", err, core.CodeTimeout)
	}
}

func TestAdapterCausesKnownToScheduler(t *testing.T) {
	expired, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	live := context.Background()
	tlsErr := fmt.Errorf("connect: %w", &tls.CertificateVerificationError{Err: x509.UnknownAuthorityError{}})
	plain := errors.New("driver text")
	errs := map[string]error{
		"preflight timed out":    preflightError(expired, core.PreflightQueryFailed, plain),
		"preflight tls":          preflightError(live, core.PreflightQueryFailed, tlsErr),
		"preflight query":        preflightError(live, core.PreflightQueryFailed, plain),
		"preflight connect":      preflightError(live, core.PreflightConnectFailed, plain),
		"acquire connect":        acquireError(live, plain),
		"acquire tls":            acquireError(live, tlsErr),
		"dedicated session rule": acquireError(live, fmt.Errorf("session guard: %w", plain)),
	}
	for name, err := range errs {
		var ce *core.Error
		if !errors.As(err, &ce) || !slices.Contains(core.PreflightCauses(), ce.Message) {
			t.Errorf("%s: error %v has a message that is not in core.PreflightCauses", name, err)
		}
	}
}
