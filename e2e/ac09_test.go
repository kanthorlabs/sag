//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kanthorlabs/sag/e2e/harness"
)

const (
	ac09Sub      = "ac09"
	ac09Resource = "pg13"
	ac09Major    = 13
)

func init() {
	harness.Register(harness.AC{
		ID:     "AC09",
		Phase:  harness.PhaseParallel,
		Agents: []harness.AgentSpec{{Sub: ac09Sub, Name: "AC09"}},
		Resources: []harness.ResourceSpec{{
			Name:       ac09Resource,
			Containers: []string{harness.ContainerSAG},
			PGMajor:    ac09Major,
			Agents:     []string{ac09Sub},
		}},
		Run: runAC09,
	})
}

func runAC09(t *testing.T, env harness.Env) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	res, err := env.Exec(ctx, harness.ContainerSAG, "sag", "resource", "test", ac09Resource)
	if err != nil {
		t.Fatalf("podman exec sag resource test %s: %v", ac09Resource, err)
	}
	output := res.Stdout + res.Stderr
	if res.ExitCode == 0 {
		t.Fatalf("sag resource test %s exited 0 on PostgreSQL 13; output %q", ac09Resource, output)
	}
	if !strings.Contains(output, "check version: FAIL") {
		t.Fatalf("sag resource test %s output %q; want a failed version check", ac09Resource, output)
	}

	session, err := env.Client(ctx, harness.ContainerSAG, ac09Sub)
	if err != nil {
		t.Fatalf("MCP client: %v", err)
	}
	defer session.Close()

	list, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "list_resources"})
	if err != nil {
		t.Fatalf("list_resources: %v", err)
	}
	if list.IsError {
		t.Fatalf("list_resources returned an error result: %+v", list.Content)
	}
	raw, err := json.Marshal(list.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var listed struct {
		Resources []struct {
			Name string `json:"name"`
		} `json:"resources"`
	}
	if err := json.Unmarshal(raw, &listed); err != nil {
		t.Fatalf("list_resources content %s: %v", raw, err)
	}
	for _, r := range listed.Resources {
		if r.Name == ac09Resource {
			t.Fatalf("list_resources = %s; want no %s", raw, ac09Resource)
		}
	}

	query, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "sql_query",
		Arguments: map[string]any{"resource": ac09Resource, "sql": "SELECT 1"},
	})
	if err != nil {
		t.Fatalf("sql_query: %v", err)
	}
	if code := ac09ErrorCode(query); code != "resource_unavailable" {
		t.Fatalf("sql_query on %s returned code %q (content %+v); want resource_unavailable", ac09Resource, code, query.Content)
	}
}

func ac09ErrorCode(res *mcp.CallToolResult) string {
	if !res.IsError {
		return ""
	}
	for _, c := range res.Content {
		text, ok := c.(*mcp.TextContent)
		if !ok {
			continue
		}
		var body struct {
			Code string `json:"code"`
		}
		if json.Unmarshal([]byte(text.Text), &body) == nil {
			return body.Code
		}
	}
	return ""
}
