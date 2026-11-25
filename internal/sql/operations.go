package sql

import (
	"errors"

	"github.com/kanthorlabs/sag/internal/core"
)

const (
	CapabilityName         = "sql"
	OperationListTables    = "list_tables"
	OperationDescribeTable = "describe_table"
	OperationQuery         = "query"
)

type QueryInput struct {
	SQL string `json:"sql"`
}

type ListTablesInput struct {
	Schema string `json:"schema,omitempty"`
}

type DescribeTableInput struct {
	Schema string `json:"schema,omitempty"`
	Table  string `json:"table"`
}

func Capability() core.Capability {
	return core.Capability{
		Name: CapabilityName,
		Operations: []core.Operation{
			{Name: OperationListTables},
			{Name: OperationDescribeTable},
			{Name: OperationQuery},
		},
	}
}

func listTablesText(in ListTablesInput) (string, error) {
	return "", errors.ErrUnsupported
}

func describeTableText(in DescribeTableInput) (string, error) {
	return "", errors.ErrUnsupported
}
