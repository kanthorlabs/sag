//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kanthorlabs/sag/e2e/harness"
)

const (
	ac29SubA      = "ac29a"
	ac29SubB      = "ac29b"
	ac29OwnA      = "ac29a"
	ac29OnlyB     = "only-b"
	ac29Missing   = "ac29-missing"
	ac29NotFound  = "not_found"
	ac29Message   = "resource not found"
	ac29ServeWait = 70 * time.Second
)

func init() {
	harness.Register(harness.AC{
		ID:     "AC29",
		Phase:  harness.PhaseParallel,
		Agents: []harness.AgentSpec{{Sub: ac29SubA, Name: "AC29A"}, {Sub: ac29SubB, Name: "AC29B"}},
		Resources: []harness.ResourceSpec{
			{Name: ac29OwnA, Agents: []string{ac29SubA}},
			{Name: ac29OnlyB, Agents: []string{ac29SubB}},
		},
		Run: runAC29,
	})
}

type ac29Error struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id"`
}

func ac29Listed(ctx context.Context, t *testing.T, session *mcp.ClientSession, want string) []string {
	t.Helper()
	deadline := time.Now().Add(ac29ServeWait)
	for {
		res, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "list_resources", Arguments: map[string]any{}})
		if err != nil || res.IsError {
			t.Fatalf("list_resources: %v %s", err, conformanceText(res))
		}
		var out struct {
			Resources []struct {
				Name string `json:"name"`
			} `json:"resources"`
		}
		if err := json.Unmarshal([]byte(conformanceText(res)), &out); err != nil {
			t.Fatalf("list_resources %q: %v", conformanceText(res), err)
		}
		var names []string
		for _, r := range out.Resources {
			names = append(names, r.Name)
		}
		if slices.Contains(names, want) {
			return names
		}
		if time.Now().After(deadline) {
			t.Fatalf("list_resources = %v; %s is not served after %s", names, want, ac29ServeWait)
		}
		time.Sleep(2 * time.Second)
	}
}

func ac29Call(ctx context.Context, t *testing.T, session *mcp.ClientSession, tool string, args map[string]any) ac29Error {
	t.Helper()
	res, err := session.CallTool(ctx, &mcp.CallToolParams{Name: tool, Arguments: args})
	if err != nil {
		t.Fatalf("%s %v: %v", tool, args, err)
	}
	text := conformanceText(res)
	if !res.IsError || res.StructuredContent != nil {
		t.Fatalf("%s %v = %s; want an error result without structured content", tool, args, text)
	}
	var out ac29Error
	if err := json.Unmarshal([]byte(text), &out); err != nil || len(out.RequestID) != 26 {
		t.Fatalf("%s %v error text %q: %v", tool, args, text, err)
	}
	return out
}

func runAC29(t *testing.T, env harness.Env) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	sessionB, err := env.Client(ctx, harness.ContainerSAG, ac29SubB)
	if err != nil {
		t.Fatalf("client %s: %v", ac29SubB, err)
	}
	defer sessionB.Close()
	ac29Listed(ctx, t, sessionB, ac29OnlyB)

	sessionA, err := env.Client(ctx, harness.ContainerSAG, ac29SubA)
	if err != nil {
		t.Fatalf("client %s: %v", ac29SubA, err)
	}
	defer sessionA.Close()
	if names := ac29Listed(ctx, t, sessionA, ac29OwnA); slices.Contains(names, ac29OnlyB) {
		t.Fatalf("list_resources for %s = %v; want no %s", ac29SubA, names, ac29OnlyB)
	}

	calls := []struct {
		tool string
		args map[string]any
	}{
		{"sql_query", map[string]any{"sql": "SELECT 1"}},
		{"sql_list_tables", map[string]any{}},
		{"sql_describe_table", map[string]any{"table": "pg_class", "schema": "pg_catalog"}},
	}
	for _, c := range calls {
		var bodies []ac29Error
		for _, resource := range []string{ac29OnlyB, ac29Missing} {
			args := map[string]any{"resource": resource}
			for k, v := range c.args {
				args[k] = v
			}
			got := ac29Call(ctx, t, sessionA, c.tool, args)
			if got.Code != ac29NotFound || got.Message != ac29Message || strings.Contains(got.Message, resource) {
				t.Fatalf("%s on %s for %s = %+v; want %s, %q", c.tool, resource, ac29SubA, got, ac29NotFound, ac29Message)
			}
			got.RequestID = ""
			bodies = append(bodies, got)
		}
		if bodies[0] != bodies[1] {
			t.Fatalf("%s: foreign resource %+v differs from missing resource %+v", c.tool, bodies[0], bodies[1])
		}
	}
}
