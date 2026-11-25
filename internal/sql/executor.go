package sql

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/kanthorlabs/sag/internal/core"
)

type executor struct {
	dialect Dialect
	pool    Pool
}

func NewExecutor(d Dialect, p Pool) core.Resource {
	return &executor{dialect: d, pool: p}
}

func (e *executor) Preflight(ctx context.Context) (core.PreflightReport, error) {
	return core.PreflightReport{}, errors.ErrUnsupported
}

func (e *executor) Execute(ctx context.Context, capability, operation string, input json.RawMessage, lim core.Limits) (core.Result, error) {
	return core.Result{}, errors.ErrUnsupported
}

func (e *executor) Close() error {
	return errors.ErrUnsupported
}

func RunReadOnly(ctx context.Context, d Dialect, p Pool, lim core.Limits, fn func(ctx context.Context, r CatalogReader) error) error {
	return errors.ErrUnsupported
}
