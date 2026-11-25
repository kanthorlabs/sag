package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/oklog/ulid/v2"

	"github.com/kanthorlabs/sag/internal/core"
	"github.com/kanthorlabs/sag/internal/pipeline"
	"github.com/kanthorlabs/sag/internal/sql"
)

const (
	messageInvalidArguments = "arguments must be a JSON object with a string resource"
	messageInternal         = "internal error"
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

type toolError struct {
	Code      core.Code `json:"code"`
	Message   string    `json:"message"`
	RequestID string    `json:"request_id"`
}

type resultOutput struct {
	Columns         []core.Column        `json:"columns"`
	Rows            [][]any              `json:"rows"`
	RowCount        int                  `json:"row_count"`
	Truncated       bool                 `json:"truncated"`
	TruncatedReason core.TruncatedReason `json:"truncated_reason,omitempty"`
	DurationMS      int64                `json:"duration_ms"`
	RequestID       string               `json:"request_id"`
}

func addSQLTools(srv *mcp.Server, calls caller) {
	added := map[string]bool{}
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
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id := ulid.Make().String()
		input := json.RawMessage(req.Params.Arguments)
		if len(input) == 0 {
			input = json.RawMessage(`{}`)
		}
		var target toolTarget
		if err := json.Unmarshal(input, &target); err != nil {
			return errorResult(id, core.NewError(core.CodeInvalidArgument, messageInvalidArguments))
		}
		start := time.Now()
		res, err := calls.Execute(ctx, pipeline.Request{
			ID:         id,
			Sub:        subject(req),
			Resource:   target.Resource,
			Capability: capability,
			Operation:  operation,
			Input:      input,
		})
		if err != nil {
			return errorResult(id, err)
		}
		return successResult(id, res, time.Since(start))
	}
}

func successResult(id string, res core.Result, elapsed time.Duration) (*mcp.CallToolResult, error) {
	out := resultOutput{
		Columns:         res.Columns,
		Rows:            res.Rows,
		RowCount:        len(res.Rows),
		Truncated:       res.Truncated,
		TruncatedReason: res.TruncatedReason,
		DurationMS:      elapsed.Milliseconds(),
		RequestID:       id,
	}
	if out.Columns == nil {
		out.Columns = []core.Column{}
	}
	if out.Rows == nil {
		out.Rows = [][]any{}
	}
	text, err := json.Marshal(out)
	if err != nil {
		return errorResult(id, err)
	}
	return &mcp.CallToolResult{
		Content:           []mcp.Content{&mcp.TextContent{Text: string(text)}},
		StructuredContent: json.RawMessage(text),
	}, nil
}

func errorResult(id string, err error) (*mcp.CallToolResult, error) {
	out := toolError{Code: core.CodeInternal, Message: messageInternal, RequestID: id}
	var ce *core.Error
	if errors.As(err, &ce) && ce.Code != "" && ce.Message != "" {
		out.Code, out.Message = ce.Code, ce.Message
	}
	text, mErr := json.Marshal(out)
	if mErr != nil {
		return nil, mErr
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: string(text)}},
		IsError: true,
	}, nil
}
