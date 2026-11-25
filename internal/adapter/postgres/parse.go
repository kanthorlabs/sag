package postgres

import (
	"errors"

	pg_query "github.com/pganalyze/pg_query_go/v6"
)

func parse(text string) (*pg_query.ParseResult, error) {
	return nil, errors.ErrUnsupported
}

func render(tree *pg_query.ParseResult) (string, error) {
	return "", errors.ErrUnsupported
}
