package core

import (
	"context"
	"encoding/json"
	"time"

	"github.com/kanthorlabs/sag/internal/secret"
)

type Adapter interface {
	Kind() string
	Capabilities() []Capability
	Open(ctx context.Context, cfg ResourceConfig, cred secret.Credential) (Resource, error)
}

type Resource interface {
	Preflight(ctx context.Context) (PreflightReport, error)
	Execute(ctx context.Context, capability, operation string, input json.RawMessage, lim Limits) (Result, error)
	Close() error
}

type Capability struct {
	Name       string
	Operations []Operation
}

type Operation struct {
	Name         string
	Description  string
	InputSchema  json.RawMessage
	OutputSchema json.RawMessage
}

type Limits struct {
	MaxRows        int
	MaxResultBytes int
	MaxCellBytes   int
	Timeout        time.Duration
}

type TLSMode string

const (
	TLSVerifyFull         TLSMode = "verify-full"
	TLSInsecureSkipVerify TLSMode = "insecure-skip-verify"
	TLSDisable            TLSMode = "disable"
)

type ApprovalVerifier interface {
	Verify(resource, fingerprint string) error
}

type ResourceConfig struct {
	Name                            string
	Kind                            string
	TLS                             TLSMode
	TLSCA                           []byte
	PoolSize                        int
	ConnectTimeout                  time.Duration
	LockTimeout                     time.Duration
	IdleInTransactionSessionTimeout time.Duration
	Approvals                       ApprovalVerifier
}

type PreflightCheck struct {
	Name   string
	Passed bool
	Detail string
}

const (
	PreflightConnectFailed = "preflight connect failed"
	PreflightTLSFailed     = "preflight TLS handshake failed"
	PreflightQueryFailed   = "preflight query failed"
	PreflightTimedOut      = "preflight timed out"
)

func PreflightCauses() []string {
	return []string{PreflightConnectFailed, PreflightTLSFailed, PreflightQueryFailed, PreflightTimedOut}
}

type PreflightReport struct {
	Passed   bool
	Checks   []PreflightCheck
	Warnings []string
	Hint     string
}
