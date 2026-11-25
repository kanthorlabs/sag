package postgres

import (
	"context"
	"errors"

	"github.com/kanthorlabs/sag/internal/approval"
	"github.com/kanthorlabs/sag/internal/core"
	"github.com/kanthorlabs/sag/internal/sql"
)

const (
	checkEngine  = "engine"
	enginePrefix = "PostgreSQL "
)

func (d *dialect) Preflight(ctx context.Context, conn sql.CatalogReader) (core.PreflightReport, error) {
	return core.PreflightReport{}, errors.ErrUnsupported
}

func riskyObjects(ctx context.Context, r sql.CatalogReader) (approval.Objects, error) {
	return approval.Objects{}, errors.ErrUnsupported
}
