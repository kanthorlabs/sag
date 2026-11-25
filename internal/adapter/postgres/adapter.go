package postgres

import (
	"context"

	"github.com/kanthorlabs/sag/internal/approval"
	"github.com/kanthorlabs/sag/internal/core"
	"github.com/kanthorlabs/sag/internal/secret"
	"github.com/kanthorlabs/sag/internal/sql"
)

const Kind = "postgres"

var (
	_ sql.Opener         = Adapter{}
	_ approval.Inspector = Adapter{}
)

func init() {
	core.Register(Adapter{})
}

type Adapter struct{}

func (Adapter) Kind() string {
	return Kind
}

func (Adapter) Capabilities() []core.Capability {
	return []core.Capability{sql.Capability()}
}

func (Adapter) Open(ctx context.Context, cfg core.ResourceConfig, cred secret.Credential) (core.Resource, error) {
	cc, err := connConfig(cfg, cred)
	if err != nil {
		return nil, err
	}
	return &resource{cfg: cfg, connConfig: cc}, nil
}

func (Adapter) OpenSQL(ctx context.Context, cfg core.ResourceConfig, cred secret.Credential) (sql.Dialect, sql.Pool, error) {
	d, p, err := openSQL(ctx, cfg, cred)
	if err != nil {
		return nil, nil, err
	}
	return d, p, nil
}

func (Adapter) Inspect(ctx context.Context, cfg core.ResourceConfig, cred secret.Credential) (approval.Target, approval.Objects, error) {
	d, p, err := openSQL(ctx, cfg, cred)
	if err != nil {
		return approval.Target{}, approval.Objects{}, err
	}
	defer p.Close()
	var objects approval.Objects
	err = sql.RunReadOnly(ctx, d, p, core.Limits{Timeout: sql.CatalogTimeout}, func(ctx context.Context, r sql.CatalogReader) error {
		var err error
		objects, err = riskyObjects(ctx, r)
		return err
	})
	if err != nil {
		return approval.Target{}, approval.Objects{}, err
	}
	return d.target, objects, nil
}

func openSQL(ctx context.Context, cfg core.ResourceConfig, cred secret.Credential) (*dialect, sql.Pool, error) {
	cc, err := connConfig(cfg, cred)
	if err != nil {
		return nil, nil, err
	}
	d := newDialect(cfg, approval.Target{
		Resource: cfg.Name,
		Host:     cc.Host,
		Port:     cc.Port,
		Database: cc.Database,
		User:     cc.User,
	})
	p, err := newPool(ctx, cc, cfg)
	if err != nil {
		return nil, nil, err
	}
	return d, p, nil
}
