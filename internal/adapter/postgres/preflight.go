package postgres

import (
	"context"
	"errors"

	"github.com/kanthorlabs/sag/internal/core"
)

func (r *resource) Preflight(ctx context.Context) (core.PreflightReport, error) {
	return core.PreflightReport{}, errors.ErrUnsupported
}
