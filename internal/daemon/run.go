package daemon

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"github.com/kanthorlabs/sag/internal/config"
	"github.com/kanthorlabs/sag/internal/endpoint"
	"github.com/kanthorlabs/sag/internal/inventory"
	"github.com/kanthorlabs/sag/internal/keys"
	"github.com/kanthorlabs/sag/internal/mcpserver"
	"github.com/kanthorlabs/sag/internal/pipeline"
	"github.com/kanthorlabs/sag/internal/secret"
)

var (
	errEnv         = errors.New("remove PG environment variables")
	errCoreLimit   = errors.New("set RLIMIT_CORE")
	errConfig      = errors.New("load config")
	errSecrets     = errors.New("load secrets")
	errMasterKey   = errors.New("master key")
	errJWTKey      = errors.New("derive JWT key")
	errCertificate = errors.New("endpoint certificate")
	errResource    = errors.New("resource")
	errMCP         = errors.New("mcp server")
	errEndpoint    = errors.New("endpoint")
)

func stageError(stage, err error) error {
	return fmt.Errorf("%w: %w", stage, err)
}

func ApplyProcessRules() error {
	if err := removePGEnv(); err != nil {
		return stageError(errEnv, err)
	}
	if err := disableCoreDumps(); err != nil {
		return stageError(errCoreLimit, err)
	}
	return nil
}

func Run(ctx context.Context, opts Options) error {
	if err := ApplyProcessRules(); err != nil {
		return err
	}
	log := slog.New(slog.NewJSONHandler(opts.Log, nil))
	log.Info("sag starting", "version", opts.Version)

	cfg, err := config.Load(opts.ConfigPath)
	if err != nil {
		return stageError(errConfig, err)
	}
	names := cfg.SecretNames()
	if !slices.Contains(names, secret.MasterKeyName) {
		names = append(names, secret.MasterKeyName)
	}
	creds, err := secret.Load(opts.SecretsDir, opts.SecretUID, names)
	if err != nil {
		return stageError(errSecrets, err)
	}
	master, err := secret.DecodeMasterKey(creds[secret.MasterKeyName])
	if err != nil {
		return stageError(errMasterKey, err)
	}
	jwtKey, err := keys.JWTKey(master, cfg.Auth.Version)
	if err != nil {
		return stageError(errJWTKey, err)
	}
	cert, err := tls.X509KeyPair(creds[string(cfg.Server.TLSCert)].Reveal(), creds[string(cfg.Server.TLSKey)].Reveal())
	if err != nil {
		return stageError(errCertificate, err)
	}

	inv := inventory.New()
	opened, err := openResources(ctx, log, cfg, creds, inv)
	defer closeResources(log, opened)
	if err != nil {
		return stageError(errResource, err)
	}

	mcpSrv, err := mcpserver.New(mcpserver.Options{
		Inventory: inv,
		Pipeline:  pipeline.New(pipeline.Options{Inventory: inv}),
		Version:   opts.Version,
	})
	if err != nil {
		return stageError(errMCP, err)
	}
	srv, err := endpoint.New(endpoint.Options{
		Listen:         cfg.Server.Listen,
		Certificate:    cert,
		AllowedOrigins: cfg.Server.AllowedOrigins,
		JWTKey:         jwtKey,
		MCP:            mcpserver.Handler(mcpSrv),
		Now:            time.Now,
		Logger:         log,
	})
	if err != nil {
		return stageError(errEndpoint, err)
	}
	log.Info("sag serving", "listen", cfg.Server.Listen)
	if err := srv.Run(ctx); err != nil {
		return stageError(errEndpoint, err)
	}
	return nil
}
