//go:build e2e

package e2e

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kanthorlabs/sag/e2e/harness"
)

func init() {
	harness.Register(harness.AC{
		ID:        "Harness_Skeleton",
		Phase:     harness.PhaseParallel,
		Agents:    []harness.AgentSpec{{Sub: "skeleton", Name: "Harness skeleton"}},
		Resources: []harness.ResourceSpec{{Name: "main", Agents: []string{"skeleton"}}},
		Run:       runHarnessSkeleton,
	})
}

func runHarnessSkeleton(t *testing.T, env harness.Env) {
	ctx := t.Context()
	session, err := env.Client(ctx, harness.ContainerSAG, "skeleton")
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	res, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "list_resources"})
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("list_resources returned an error result: %+v", res.Content)
	}
	raw, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var out struct {
		Resources []struct {
			Name string `json:"name"`
			Kind string `json:"kind"`
		} `json:"resources"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("structured content %s: %v", raw, err)
	}
	var names []string
	for _, r := range out.Resources {
		names = append(names, r.Name)
	}
	if !slices.Equal(names, []string{"main"}) {
		t.Errorf("list_resources names = %v, want [main]; content %s", names, raw)
	}
}
