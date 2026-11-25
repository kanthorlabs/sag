package passthrough

import (
	"context"
	"errors"

	"github.com/kanthorlabs/sag/internal/sql"
)

type dialect struct {
	sql.Dialect
}

func Wrap(d sql.Dialect) sql.Dialect {
	return dialect{Dialect: d}
}

func (d dialect) Parse(text string) (sql.Tree, error) {
	return nil, errors.ErrUnsupported
}

func (d dialect) ValidateStatic(t sql.Tree) error {
	return errors.ErrUnsupported
}

func (d dialect) Render(t sql.Tree) (string, error) {
	return "", errors.ErrUnsupported
}

func (d dialect) ValidateCatalog(ctx context.Context, tx sql.CatalogReader, t sql.Tree) error {
	return errors.ErrUnsupported
}

func (d dialect) PlanRowCap(t sql.Tree, rendered string, maxRows int) (sql.RowCapPlan, error) {
	return sql.RowCapPlan{}, errors.ErrUnsupported
}
