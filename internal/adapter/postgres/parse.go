package postgres

import (
	pg_query "github.com/pganalyze/pg_query_go/v6"

	"github.com/kanthorlabs/sag/internal/core"
)

const (
	msgParseFailed  = "statement does not parse"
	msgRenderFailed = "statement does not render"
)

func parse(text string) (*pg_query.ParseResult, error) {
	tree, err := pg_query.Parse(text)
	if err != nil {
		return nil, &core.Error{Code: core.CodeQueryRejected, Message: msgParseFailed, Cause: err}
	}
	return tree, nil
}

func render(tree *pg_query.ParseResult) (string, error) {
	if tree == nil {
		return "", core.NewError(core.CodeQueryRejected, msgRenderFailed)
	}
	text, err := pg_query.Deparse(tree)
	if err != nil {
		return "", &core.Error{Code: core.CodeQueryRejected, Message: msgRenderFailed, Cause: err}
	}
	return text, nil
}
