package mcpserver

import (
	"encoding/json"
	"errors"
	"slices"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kanthorlabs/sag/internal/core"
)

const messageInternal = "internal error"

var errorCodes = []core.Code{
	core.CodeUnauthenticated,
	core.CodeNotFound,
	core.CodeInvalidArgument,
	core.CodeQueryRejected,
	core.CodeTimeout,
	core.CodeRateLimited,
	core.CodeResourceUnavailable,
	core.CodeUpstreamError,
	core.CodeInternal,
}

func errorResult(id string, err error) (*mcp.CallToolResult, error) {
	out := ErrorResult{Code: core.CodeInternal, Message: messageInternal, RequestID: id}
	var ce *core.Error
	if errors.As(err, &ce) && slices.Contains(errorCodes, ce.Code) && ce.Message != "" {
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

func successResult(id string, out any) (*mcp.CallToolResult, error) {
	text, err := json.Marshal(out)
	if err != nil {
		return errorResult(id, err)
	}
	return &mcp.CallToolResult{
		Content:           []mcp.Content{&mcp.TextContent{Text: string(text)}},
		StructuredContent: json.RawMessage(text),
	}, nil
}
