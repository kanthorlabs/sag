package postgres

import (
	"fmt"

	pg_query "github.com/pganalyze/pg_query_go/v6"

	"github.com/kanthorlabs/sag/internal/core"
	"github.com/kanthorlabs/sag/internal/sql"
)

const (
	cursorName      = "sag_c"
	msgRowCapFailed = "statement has no row cap plan"
	declarePrefix   = "DECLARE " + cursorName + " NO SCROLL CURSOR FOR "
	fetchFormat     = "FETCH %d FROM " + cursorName
)

func planRowCap(t *pgTree, rendered string, maxRows int) (sql.RowCapPlan, error) {
	if maxRows < 1 {
		return sql.RowCapPlan{}, rowCapRejected()
	}
	stmt, ok := singleStatement(t.Result)
	if !ok {
		return sql.RowCapPlan{}, rowCapRejected()
	}
	switch stmt.Node.(type) {
	case *pg_query.Node_ExplainStmt:
		return sql.RowCapPlan{Statement: rendered}, nil
	case *pg_query.Node_SelectStmt:
		statement := declarePrefix + rendered
		if !isSingleDeclare(statement) {
			return sql.RowCapPlan{}, rowCapRejected()
		}
		return sql.RowCapPlan{Statement: statement, Fetch: fmt.Sprintf(fetchFormat, maxRows+1)}, nil
	default:
		return sql.RowCapPlan{}, rowCapRejected()
	}
}

func singleStatement(tree *pg_query.ParseResult) (*pg_query.Node, bool) {
	if tree == nil || len(tree.GetStmts()) != 1 {
		return nil, false
	}
	stmt := tree.GetStmts()[0].GetStmt()
	return stmt, stmt != nil
}

func isSingleDeclare(statement string) bool {
	tree, err := pg_query.Parse(statement)
	if err != nil {
		return false
	}
	stmt, ok := singleStatement(tree)
	if !ok {
		return false
	}
	declare := stmt.GetDeclareCursorStmt()
	return declare != nil && declare.GetPortalname() == cursorName
}

func rowCapRejected() error {
	return core.NewError(core.CodeQueryRejected, msgRowCapFailed)
}
