//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kanthorlabs/sag/e2e/harness"
)

const (
	ac27Sub      = "ac27"
	ac27Resource = "ac27"
)

func init() {
	harness.Register(harness.AC{
		ID:     "AC27",
		Phase:  harness.PhaseParallel,
		Agents: []harness.AgentSpec{{Sub: ac27Sub, Name: "AC27"}},
		Resources: []harness.ResourceSpec{{
			Name:       ac27Resource,
			Containers: []string{harness.ContainerROAudit},
			Agents:     []string{ac27Sub},
		}},
		Run: runAC27,
	})
}

func runAC27(t *testing.T, env harness.Env) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	session, err := env.Client(ctx, harness.ContainerROAudit, ac27Sub)
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	defer session.Close()

	for _, call := range []*mcp.CallToolParams{
		{Name: "sql_query", Arguments: map[string]any{"resource": ac27Resource, "sql": "SELECT 27 AS n"}},
		{Name: "list_resources", Arguments: map[string]any{}},
	} {
		res, err := session.CallTool(ctx, call)
		if err != nil {
			t.Fatalf("%s: %v", call.Name, err)
		}
		text := conformanceText(res)
		if !res.IsError || res.StructuredContent != nil {
			t.Fatalf("%s: IsError = %v, structured = %v; want an error with no data: %s", call.Name, res.IsError, res.StructuredContent, text)
		}
		var out map[string]any
		if err := json.Unmarshal([]byte(text), &out); err != nil {
			t.Fatalf("%s: error text %q: %v", call.Name, text, err)
		}
		if out["code"] != "internal" || out["message"] != "internal error" || len(out) != 3 {
			t.Fatalf("%s: error = %v; want internal with code, message, request_id only", call.Name, out)
		}
	}

	records, err := env.Audit(ctx, harness.ContainerROAudit)
	if err != nil {
		t.Fatalf("audit copy: %v", err)
	}
	if len(records) != 0 {
		t.Fatalf("read-only audit directory has %d records; want 0", len(records))
	}
}
