package mcpserver

import (
	"context"
	"encoding/json"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/oklog/ulid/v2"

	"github.com/kanthorlabs/sag/internal/core"
	"github.com/kanthorlabs/sag/internal/pipeline"
	"github.com/kanthorlabs/sag/internal/sql"
)

const (
	messageInvalidArguments = "arguments must be a JSON object with a string resource"
)

var registeredCapabilities = func() []core.Capability {
	var caps []core.Capability
	for _, kind := range core.Kinds() {
		if a, ok := core.Lookup(kind); ok {
			caps = append(caps, a.Capabilities()...)
		}
	}
	return caps
}

type toolTarget struct {
	Resource string `json:"resource"`
}

func addSQLTools(srv *mcp.Server, calls caller, added map[string]bool) {
	for _, c := range registeredCapabilities() {
		if c.Name != sql.CapabilityName {
			continue
		}
		for _, op := range c.Operations {
			name := c.Name + "_" + op.Name
			if added[name] {
				continue
			}
			added[name] = true
			srv.AddTool(&mcp.Tool{
				Name:         name,
				Description:  op.Description,
				InputSchema:  op.InputSchema,
				OutputSchema: op.OutputSchema,
				Annotations:  readOnlyAnnotations(),
			}, callOperation(calls, c.Name, op.Name))
		}
	}
}

func callOperation(calls caller, capability, operation string) mcp.ToolHandler {
	return func(ctx context.Context, req *mcp.CallToolRequest) (result *mcp.CallToolResult, err error) {
		preq := toolRequest(req, ulid.Make().String())
		preq.Capability = capability
		preq.Operation = operation
		audited := false
		defer func() {
			if recover() != nil {
				result, err = panicResult(calls, preq, audited)
			}
		}()
		input := json.RawMessage(req.Params.Arguments)
		if len(input) == 0 {
			input = json.RawMessage(`{}`)
		}
		preq.Input = input
		var target toolTarget
		if err := json.Unmarshal(input, &target); err != nil {
			audited = true
			return errorResult(preq.ID, calls.Reject(preq, core.NewError(core.CodeInvalidArgument, messageInvalidArguments)))
		}
		preq.Resource = target.Resource
		start := time.Now()
		res, err := calls.Execute(ctx, preq)
		audited = true
		if err != nil {
			return errorResult(preq.ID, err)
		}
		return successResult(preq.ID, operationOutput(operation, preq.ID, res, time.Since(start)))
	}
}

func panicResult(calls caller, preq pipeline.Request, audited bool) (*mcp.CallToolResult, error) {
	err := error(core.NewError(core.CodeInternal, messageInternal))
	if !audited {
		err = calls.Reject(preq, err)
	}
	return errorResult(preq.ID, err)
}

func operationOutput(operation, id string, res core.Result, elapsed time.Duration) any {
	columns, rows := res.Columns, res.Rows
	if columns == nil {
		columns = []core.Column{}
	}
	if rows == nil {
		rows = [][]any{}
	}
	if operation != sql.OperationQuery {
		return SQLCatalogOutput{Columns: columns, Rows: rows, RowCount: len(rows), Truncated: res.Truncated, RequestID: id}
	}
	return SQLQueryOutput{
		Columns:         columns,
		Rows:            rows,
		RowCount:        len(rows),
		Truncated:       res.Truncated,
		TruncatedReason: res.TruncatedReason,
		DurationMS:      elapsed.Milliseconds(),
		RequestID:       id,
	}
}
