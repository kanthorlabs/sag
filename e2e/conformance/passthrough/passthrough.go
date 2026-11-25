package passthrough

import (
	"context"
	"errors"

	"github.com/kanthorlabs/sag/internal/sql"
)

var errNotText = errors.New("passthrough: tree is not text")

type dialect struct {
	sql.Dialect
}

func Wrap(d sql.Dialect) sql.Dialect {
	return dialect{Dialect: d}
}

func (d dialect) Parse(text string) (sql.Tree, error) {
	return text, nil
}

func (d dialect) ValidateStatic(t sql.Tree) error {
	return nil
}

func (d dialect) Render(t sql.Tree) (string, error) {
	text, ok := t.(string)
	if !ok {
		return "", errNotText
	}
	return text, nil
}

func (d dialect) ValidateCatalog(ctx context.Context, tx sql.CatalogReader, t sql.Tree) error {
	return nil
}

func (d dialect) PlanRowCap(t sql.Tree, rendered string, maxRows int) (sql.RowCapPlan, error) {
	return sql.RowCapPlan{Statement: rendered}, nil
}
