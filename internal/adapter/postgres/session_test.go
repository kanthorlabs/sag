package postgres

import (
	"slices"
	"testing"
	"time"

	"github.com/kanthorlabs/sag/internal/approval"
	"github.com/kanthorlabs/sag/internal/core"
)

func TestSessionGuardText(t *testing.T) {
	want := []string{"SET default_transaction_read_only = on"}
	if got := sessionGuard(); !slices.Equal(got, want) {
		t.Fatalf("sessionGuard() = %q; want %q", got, want)
	}
	if got := newDialect(core.ResourceConfig{}, approval.Target{}).SessionGuard(); !slices.Equal(got, want) {
		t.Fatalf("dialect SessionGuard() = %q; want %q", got, want)
	}
}

func TestRequestBeginText(t *testing.T) {
	cfg := core.ResourceConfig{LockTimeout: 2 * time.Second, IdleInTransactionSessionTimeout: 20 * time.Second}
	lim := core.Limits{Timeout: 15 * time.Second}
	want := []string{
		"BEGIN TRANSACTION READ ONLY",
		"SET LOCAL statement_timeout = 15000",
		"SET LOCAL lock_timeout = 2000",
		"SET LOCAL idle_in_transaction_session_timeout = 20000",
		"SET LOCAL standard_conforming_strings = on",
		"SET LOCAL client_encoding = 'UTF8'",
		"SET LOCAL datestyle = 'ISO, YMD'",
		"SET LOCAL bytea_output = 'hex'",
	}
	if got := requestBegin(lim, cfg); !slices.Equal(got, want) {
		t.Fatalf("requestBegin() = %q; want %q", got, want)
	}
	if got := newDialect(cfg, approval.Target{}).RequestBegin(lim); !slices.Equal(got, want) {
		t.Fatalf("dialect RequestBegin() = %q; want %q", got, want)
	}
}

func TestRequestBeginIdleTimeoutFromLimits(t *testing.T) {
	cfg := core.ResourceConfig{LockTimeout: 2 * time.Second, IdleInTransactionSessionTimeout: 20 * time.Second}
	got := requestBegin(core.Limits{Timeout: 30 * time.Second, IdleTimeout: 35 * time.Second}, cfg)
	if got[3] != "SET LOCAL idle_in_transaction_session_timeout = 35000" {
		t.Fatalf("idle line = %q; want the request limit", got[3])
	}
	got = requestBegin(core.Limits{Timeout: 10 * time.Second}, cfg)
	if got[3] != "SET LOCAL idle_in_transaction_session_timeout = 20000" {
		t.Fatalf("idle line = %q; want the resource value when the limit is zero", got[3])
	}
}

func TestRequestBeginTextNeverDisablesTimeouts(t *testing.T) {
	cfg := core.ResourceConfig{LockTimeout: 0, IdleInTransactionSessionTimeout: 1500 * time.Microsecond}
	got := requestBegin(core.Limits{Timeout: -time.Second}, cfg)
	want := []string{
		"BEGIN TRANSACTION READ ONLY",
		"SET LOCAL statement_timeout = 1",
		"SET LOCAL lock_timeout = 1",
		"SET LOCAL idle_in_transaction_session_timeout = 2",
		"SET LOCAL standard_conforming_strings = on",
		"SET LOCAL client_encoding = 'UTF8'",
		"SET LOCAL datestyle = 'ISO, YMD'",
		"SET LOCAL bytea_output = 'hex'",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("requestBegin() = %q; want %q", got, want)
	}
}
