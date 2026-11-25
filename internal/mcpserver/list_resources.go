package mcpserver

import (
	"context"
	"slices"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kanthorlabs/sag/internal/inventory"
)

type ResourceInfo struct {
	Name         string   `json:"name"`
	Kind         string   `json:"kind"`
	Description  string   `json:"description"`
	Capabilities []string `json:"capabilities"`
}

type ListResourcesOutput struct {
	Resources []ResourceInfo `json:"resources"`
}

func addListResources(srv *mcp.Server, inv visibility) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "list_resources",
		Description: "List the resources that this agent can query, with kind, description, and capabilities. " + untrustedResults,
		Annotations: readOnlyAnnotations(),
	}, listResources(inv))
}

func listResources(inv visibility) mcp.ToolHandlerFor[struct{}, ListResourcesOutput] {
	return func(_ context.Context, req *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, ListResourcesOutput, error) {
		out := ListResourcesOutput{Resources: []ResourceInfo{}}
		sub := subject(req)
		if sub == "" {
			return nil, out, nil
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
		return nil, out, nil
	}
}
