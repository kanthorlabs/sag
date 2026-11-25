package postgres

import (
	"errors"

	pg_query "github.com/pganalyze/pg_query_go/v6"
)

func validateStatic(tree *pg_query.ParseResult) error {
	return errors.ErrUnsupported
}
