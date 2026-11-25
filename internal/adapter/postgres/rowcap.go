package postgres

import (
	"fmt"
	"math"

	pg_query "github.com/pganalyze/pg_query_go/v6"

	"github.com/kanthorlabs/sag/internal/core"
	"github.com/kanthorlabs/sag/internal/sql"
)

const (
	cursorName      = "sag_c"
	msgRowCapFailed = "statement has no row cap plan"
	declarePrefix   = "DECLARE " + cursorName + " NO SCROLL CURSOR FOR "
	fetchFormat     = "FETCH %d FROM " + cursorName
	maxFetchRows    = math.MaxInt32 - 1
)

func planRowCap(t *pgTree, rendered string, maxRows int) (sql.RowCapPlan, error) {
	if t == nil || t.Result == nil || !t.Validated {
		return sql.RowCapPlan{}, core.NewError(core.CodeQueryRejected, msgTreeNotValidated)
	}
	if maxRows < 1 || maxRows > maxFetchRows {
		return sql.RowCapPlan{}, rowCapRejected()
	}
	stmt, ok := singleStatement(t.Result)
	if !ok {
		return sql.RowCapPlan{}, rowCapRejected()
	}
	switch stmt.Node.(type) {
	case *pg_query.Node_ExplainStmt:
		if !rendersTree(rendered, t.Result) {
			return sql.RowCapPlan{}, rowCapRejected()
		}
		return sql.RowCapPlan{Statement: rendered}, nil
	case *pg_query.Node_SelectStmt:
		statement := declarePrefix + rendered
		if !declaresQuery(statement, stmt) {
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

func rendersTree(text string, tree *pg_query.ParseResult) bool {
	if checkLexerIndependent(text) != nil {
		return false
	}
	again, err := pg_query.Parse(text)
	if err != nil {
		return false
	}
	same, err := sameTree(tree, again)
	return err == nil && same
}

func declaresQuery(statement string, query *pg_query.Node) bool {
	if checkLexerIndependent(statement) != nil {
		return false
	}
	tree, err := pg_query.Parse(statement)
	if err != nil {
		return false
	}
	stmt, ok := singleStatement(tree)
	if !ok {
		return false
	}
	declare := stmt.GetDeclareCursorStmt()
	if declare == nil || declare.GetPortalname() != cursorName || declare.GetQuery() == nil {
		return false
	}
	same, err := sameTree(query, declare.GetQuery())
	return err == nil && same
}

func rowCapRejected() error {
	return core.NewError(core.CodeQueryRejected, msgRowCapFailed)
}
