package postgres

import (
	"context"
	"errors"

	"github.com/kanthorlabs/sag/internal/approval"
	"github.com/kanthorlabs/sag/internal/core"
	"github.com/kanthorlabs/sag/internal/sql"
)

const sqlStateReadOnly = "25006"

var _ sql.Dialect = (*dialect)(nil)

type dialect struct {
	cfg    core.ResourceConfig
	target approval.Target
}

func newDialect(cfg core.ResourceConfig, target approval.Target) *dialect {
	return &dialect{cfg: cfg, target: target}
}

func (d *dialect) EngineGuard() sql.EngineGuard {
	return sql.EngineGuardTransaction
}

func (d *dialect) SessionGuard() []string {
	return nil
}

func (d *dialect) RequestBegin(lim core.Limits) []string {
	return nil
}

func (d *dialect) Parse(text string) (sql.Tree, error) {
	return nil, errors.ErrUnsupported
}

func (d *dialect) ValidateStatic(t sql.Tree) error {
	return errors.ErrUnsupported
}

func (d *dialect) Render(t sql.Tree) (string, error) {
	return "", errors.ErrUnsupported
}

func (d *dialect) ValidateCatalog(ctx context.Context, tx sql.CatalogReader, t sql.Tree) error {
	return errors.ErrUnsupported
}

func (d *dialect) PlanRowCap(t sql.Tree, rendered string, maxRows int) (sql.RowCapPlan, error) {
	return sql.RowCapPlan{}, errors.ErrUnsupported
}

func (d *dialect) ViolationCodes() []string {
	return []string{sqlStateReadOnly}
}
