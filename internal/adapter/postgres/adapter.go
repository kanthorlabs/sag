package postgres

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/kanthorlabs/sag/internal/core"
	"github.com/kanthorlabs/sag/internal/secret"
)

const Kind = "postgres"

func init() {
	core.Register(Adapter{})
}

type Adapter struct{}

func (Adapter) Kind() string {
	return Kind
}

func (Adapter) Capabilities() []core.Capability {
	return nil
}

func (Adapter) Open(ctx context.Context, cfg core.ResourceConfig, cred secret.Credential) (core.Resource, error) {
	cc, err := connConfig(cfg, cred)
	if err != nil {
		return nil, err
	}
	return &resource{cfg: cfg, connConfig: cc}, nil
}

type resource struct {
	cfg        core.ResourceConfig
	connConfig *pgx.ConnConfig
}

func (r *resource) Execute(ctx context.Context, capability, operation string, input json.RawMessage, lim core.Limits) (core.Result, error) {
	return core.Result{}, errors.ErrUnsupported
}

func (r *resource) Close() error {
	return nil
}
