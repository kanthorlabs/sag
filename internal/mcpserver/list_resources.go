package mcpserver

import (
	"context"
	"encoding/json"
	"slices"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/oklog/ulid/v2"

	"github.com/kanthorlabs/sag/internal/core"
	"github.com/kanthorlabs/sag/internal/inventory"
)

const (
	listResourcesName          = "list_resources"
	messageListResourcesNoArgs = "arguments must be an empty JSON object"
)

func addListResources(srv *mcp.Server, inv visibility, calls caller) {
	srv.AddTool(&mcp.Tool{
		Name:         listResourcesName,
		Description:  "List the resources that this agent can query, with kind, description, and capabilities. " + untrustedResults,
		InputSchema:  listResourcesInputSchema,
		OutputSchema: listResourcesOutputSchema,
		Annotations:  readOnlyAnnotations(),
	}, listResources(inv, calls))
}

func listResources(inv visibility, calls caller) mcp.ToolHandler {
	return func(_ context.Context, req *mcp.CallToolRequest) (result *mcp.CallToolResult, err error) {
		preq := toolRequest(req, ulid.Make().String())
		preq.Operation = listResourcesName
		audited := false
		defer func() {
			if recover() != nil {
				result, err = panicResult(calls, preq, audited)
			}
		}()
		if !emptyArguments(req.Params.Arguments) {
			audited = true
			return errorResult(preq.ID, calls.Reject(preq, core.NewError(core.CodeInvalidArgument, messageListResourcesNoArgs)))
		}
		out := visibleResources(inv, preq.Sub)
		audited = true
		if err := calls.Listed(preq, len(out.Resources)); err != nil {
			return errorResult(preq.ID, err)
		}
		return successResult(preq.ID, out)
	}
}

func emptyArguments(raw json.RawMessage) bool {
	if len(raw) == 0 || string(raw) == "null" {
		return true
	}
	var args map[string]json.RawMessage
	return json.Unmarshal(raw, &args) == nil && args != nil && len(args) == 0
}

func visibleResources(inv visibility, sub string) ListResourcesOutput {
	out := ListResourcesOutput{Resources: []ResourceInfo{}}
	if sub == "" {
		return out
	}
	for _, e := range inv.Visible(sub) {
		if e.Status != inventory.StatusServed || !slices.Contains(e.Agents, sub) {
			continue
		}
		out.Resources = append(out.Resources, ResourceInfo{
			Name:         e.Name,
			Kind:         e.Kind,
			Description:  e.Description,
			Capabilities: append([]string{}, e.Capabilities...),
		})
	}
	return out
}
