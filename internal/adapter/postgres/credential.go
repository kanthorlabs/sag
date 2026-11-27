package postgres

import (
	"context"
	"errors"
	"net"

	"github.com/jackc/pgx/v5"
	"github.com/kanthorlabs/sag/internal/core"
	"github.com/kanthorlabs/sag/internal/secret"
	"github.com/kanthorlabs/sag/internal/sql"
)

var (
	errCheckConnect = errors.New("credential check: cannot connect")
	errCheckEngine  = errors.New("credential check: engine is not PostgreSQL")
	errCheckVersion = errors.New("credential check: version is not supported")
)

var _ core.CredentialKind = Adapter{}

func (Adapter) CredentialPrompt() string {
	return "PostgreSQL DSN"
}

func (Adapter) ValidateCredential(cred secret.Credential) (core.CredentialNotes, error) {
	f, err := parseDSN(string(cred.Reveal()))
	if err != nil {
		return core.CredentialNotes{}, err
	}
	return core.CredentialNotes{LoopbackHost: isLoopbackHost(f.host)}, nil
}

func isLoopbackHost(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func (Adapter) CheckCredential(ctx context.Context, cfg core.ResourceConfig, cred secret.Credential) error {
	if cfg.PoolSize < 1 {
		cfg.PoolSize = 1
	}
	cc, err := connConfig(cfg, cred)
	if err != nil {
		return errCheckConnect
	}
	pgxConn, err := pgx.ConnectConfig(ctx, cc)
	if err != nil {
		return errCheckConnect
	}
	closeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), closeTimeout)
	defer cancel()
	defer pgxConn.Close(closeCtx)
	return credentialCheckServer(ctx, &conn{pc: pgxConn})
}

func credentialCheckServer(ctx context.Context, r sql.CatalogReader) error {
	if err := pinSearchPath(ctx, r); err != nil {
		return errCheckConnect
	}
	engine, version, err := checkServer(ctx, r)
	if err != nil {
		return errCheckConnect
	}
	if !engine.Passed {
		return errCheckEngine
	}
	if !version.Passed {
		return errCheckVersion
	}
	return nil
}
