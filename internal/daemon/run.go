package daemon

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"path/filepath"
	"slices"
	"time"

	"github.com/kanthorlabs/sag/internal/approval"
	"github.com/kanthorlabs/sag/internal/audit"
	"github.com/kanthorlabs/sag/internal/config"
	"github.com/kanthorlabs/sag/internal/endpoint"
	"github.com/kanthorlabs/sag/internal/inventory"
	"github.com/kanthorlabs/sag/internal/keys"
	"github.com/kanthorlabs/sag/internal/mcpserver"
	"github.com/kanthorlabs/sag/internal/pipeline"
	"github.com/kanthorlabs/sag/internal/preflight"
	"github.com/kanthorlabs/sag/internal/scrub"
	"github.com/kanthorlabs/sag/internal/secret"
	"github.com/kanthorlabs/sag/internal/secret/awssm"
)

var (
	errEnv         = errors.New("remove PG environment variables")
	errCoreLimit   = errors.New("set RLIMIT_CORE")
	errConfig      = errors.New("load config")
	errSecrets     = errors.New("load secrets")
	errMasterKey   = errors.New("master key")
	errJWTKey      = errors.New("derive JWT key")
	errApprovalKey = errors.New("derive approval key")
	errAudit       = errors.New("audit directory")
	errCertificate = errors.New("endpoint certificate")
	errResource    = errors.New("resource")
	errMCP         = errors.New("mcp server")
	errEndpoint    = errors.New("endpoint")
)

const passThroughWarning = "pass-through dialect active: test-only conformance build without C1, not a product artifact"

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
	if opts.WrapDialect != nil {
		log.Warn(passThroughWarning)
	}

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
	scrubber := scrub.New(slices.Collect(maps.Values(creds)))
	if awsCred, ok := creds[string(cfg.Storage.AWS.Credentials)]; ok && cfg.Storage.AWS.Credentials != "" {
		scrubber.Add(awsCred)
	}
	log = slog.New(scrubHandler(log.Handler(), scrubber))
	fatal := func(stage, err error) error {
		return stageError(stage, scrubber.Error(err))
	}
	master, err := secret.DecodeMasterKey(creds[secret.MasterKeyName])
	if err != nil {
		return fatal(errMasterKey, err)
	}
	jwtKey, err := keys.JWTKey(master, cfg.Auth.Version)
	if err != nil {
		return fatal(errJWTKey, err)
	}
	cert, err := tls.X509KeyPair(creds[string(cfg.Server.TLSCert)].Reveal(), creds[string(cfg.Server.TLSKey)].Reveal())
	if err != nil {
		return fatal(errCertificate, err)
	}

	approvalKey, err := keys.ApprovalKey(master)
	if err != nil {
		return fatal(errApprovalKey, err)
	}
	approvals := approval.NewStore(filepath.Join(opts.StateDir, approval.DirName), approvalKey)
	auditWriter, err := audit.NewFileWriter(filepath.Join(opts.StateDir, audit.DirName))
	if err != nil {
		return fatal(errAudit, err)
	}

	var loader awsLoader
	if cfg.Storage.AWS.Region != "" {
		if opts.testAWSLoader != nil {
			loader = opts.testAWSLoader
		} else {
			b, aerr := awssm.NewDaemon(ctx, cfg.Storage.AWS.Region, string(cfg.Storage.AWS.Credentials))
			if aerr != nil {
				return fatal(errResource, fmt.Errorf("aws backend: %w", aerr))
			}
			loader = b
		}
	}

	inv := inventory.New()
	sched := preflight.New(preflight.Options{
		Clock:           preflight.SystemClock(),
		Inventory:       inv,
		Log:             log,
		RecheckInterval: cfg.Preflight.RecheckInterval,
	})
	opened, err := openResources(ctx, log, cfg, creds, loader, scrubber, inv, approvals, opts.WrapDialect, sched)
	defer closeResources(log, opened)
	if err != nil {
		return fatal(errResource, err)
	}
	ctx, cancel := context.WithCancel(ctx)
	defer sched.Wait()
	defer cancel()
	sched.Start(ctx)

	mcpSrv, err := mcpserver.New(mcpserver.Options{
		Inventory: inv,
		Pipeline: pipeline.New(pipeline.Options{
			Inventory:             inv,
			MaxConcurrentPerAgent: cfg.Defaults.Limits.MaxConcurrentPerAgent,
			Scrubber:              scrubber,
			Audit:                 auditWriter,
			Log:                   log,
		}),
		Version: opts.Version,
	})
	if err != nil {
		return fatal(errMCP, err)
	}
	srv, err := endpoint.New(endpoint.Options{
		Listen:         cfg.Server.Listen,
		Certificate:    cert,
		AllowedOrigins: cfg.Server.AllowedOrigins,
		JWTKey:         jwtKey,
		MCP:            mcpserver.Handler(mcpSrv),
		Now:            time.Now,
		Logger:         log,
		Audit:          authAuditHook(auditWriter, log),
	})
	if err != nil {
		return fatal(errEndpoint, err)
	}
	log.Info("sag serving", "listen", cfg.Server.Listen)
	if err := srv.Run(ctx); err != nil {
		return fatal(errEndpoint, err)
	}
	return nil
}
