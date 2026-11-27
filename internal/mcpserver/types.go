package mcpserver

import "github.com/kanthorlabs/sag/internal/core"

type ListResourcesInput struct{}

type ResourceInfo struct {
	Name         string   `json:"name"`
	Kind         string   `json:"kind"`
	Description  string   `json:"description"`
	Capabilities []string `json:"capabilities"`
}

type ListResourcesOutput struct {
	Resources []ResourceInfo `json:"resources"`
}

type SQLListTablesInput struct {
	Resource string `json:"resource"`
	Schema   string `json:"schema,omitempty"`
}

type SQLDescribeTableInput struct {
	Resource string `json:"resource"`
	Schema   string `json:"schema,omitempty"`
	Table    string `json:"table"`
}

type SQLQueryInput struct {
	Resource       string `json:"resource"`
	SQL            string `json:"sql"`
	MaxRows        *int   `json:"max_rows,omitempty"`
	MaxResultBytes *int   `json:"max_result_bytes,omitempty"`
	TimeoutSeconds *int   `json:"timeout_seconds,omitempty"`
}

type SQLCatalogOutput struct {
	Columns   []core.Column `json:"columns"`
	Rows      [][]any       `json:"rows"`
	RowCount  int           `json:"row_count"`
	Truncated bool          `json:"truncated"`
	RequestID string        `json:"request_id"`
}

type SQLListTablesOutput = SQLCatalogOutput

type SQLDescribeTableOutput = SQLCatalogOutput

type SQLQueryOutput struct {
	Columns         []core.Column        `json:"columns"`
	Rows            [][]any              `json:"rows"`
	RowCount        int                  `json:"row_count"`
	Truncated       bool                 `json:"truncated"`
	TruncatedReason core.TruncatedReason `json:"truncated_reason,omitempty"`
	DurationMS      int64                `json:"duration_ms"`
	RequestID       string               `json:"request_id"`
}

type ErrorResult struct {
	Code      core.Code `json:"code"`
	Message   string    `json:"message"`
	RequestID string    `json:"request_id"`
}
