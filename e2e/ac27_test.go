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
	ac27Sub      = "ac27"
	ac27Resource = "ac27"
	ac27SQL      = "SELECT 27 AS n"
	ac27Logged   = "27 AS n"
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

	ac27WaitServed(ctx, t, env)
	start := time.Now()
	for _, call := range []*mcp.CallToolParams{
		{Name: "sql_query", Arguments: map[string]any{"resource": ac27Resource, "sql": ac27SQL}},
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

	logs, err := env.Postgres(env.Major()).Logs(ctx, start.Add(-time.Second))
	if err != nil {
		t.Fatalf("PostgreSQL logs: %v", err)
	}
	if !strings.Contains(logs, ac27Logged) {
		t.Fatalf("PostgreSQL log has no statement with %q; the query did not run", ac27Logged)
	}
}

func ac27WaitServed(ctx context.Context, t *testing.T, env harness.Env) {
	t.Helper()
	deadline := time.Now().Add(70 * time.Second)
	for {
		logs, err := env.Logs(ctx, harness.ContainerROAudit)
		if err != nil {
			t.Fatalf("logs: %v", err)
		}
		for _, line := range strings.Split(logs, "\n") {
			var l struct {
				Msg      string `json:"msg"`
				Resource string `json:"resource"`
			}
			if json.Unmarshal([]byte(strings.TrimSpace(line)), &l) == nil && l.Msg == "resource served" && l.Resource == ac27Resource {
				return
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("resource %s is not served after 70 s:\n%s", ac27Resource, logs)
		}
		select {
		case <-ctx.Done():
			t.Fatalf("wait for served state: %v", ctx.Err())
		case <-time.After(500 * time.Millisecond):
		}
	}
}
