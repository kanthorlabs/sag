package postgres

import (
	pg_query "github.com/pganalyze/pg_query_go/v6"

	"github.com/kanthorlabs/sag/internal/core"
)

const msgStatementNotAllowed = "statement type is not allowed"

func validateStatic(tree *pg_query.ParseResult) error {
	return core.NewError(core.CodeQueryRejected, msgStatementNotAllowed)
}
