package postgres

import (
	"errors"

	pg_query "github.com/pganalyze/pg_query_go/v6"

	"github.com/kanthorlabs/sag/internal/sql"
)

func planRowCap(tree *pg_query.ParseResult, rendered string, maxRows int) (sql.RowCapPlan, error) {
	return sql.RowCapPlan{}, errors.ErrUnsupported
}
