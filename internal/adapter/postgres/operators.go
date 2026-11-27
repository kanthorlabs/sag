package postgres

import (
	"context"
	"slices"

	"github.com/kanthorlabs/sag/internal/sql"
)

const queryOperators = `SELECT o.oprname FROM pg_catalog.pg_operator o JOIN pg_catalog.pg_namespace n ON n.oid = o.oprnamespace WHERE n.nspname <> 'pg_catalog'`

func operatorCollisions(ctx context.Context, r sql.CatalogReader) ([]string, error) {
	rows, err := queryRows(ctx, r, queryOperators)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(rows))
	for _, row := range rows {
		name, err := textColumn(row, 0)
		if err != nil {
			return nil, err
		}
		names = append(names, name)
	}
	slices.Sort(names)
	return slices.Compact(names), nil
}
