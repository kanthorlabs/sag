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
	ac20Sub         = "ac20"
	ac20Resource    = "ac20"
	ac20Database    = "sag_ac20"
	ac20Select      = "SELECT id FROM t10k ORDER BY id"
	ac20ServeWindow = 70 * time.Second
)

func init() {
	harness.Register(harness.AC{
		ID:     "AC20",
		Phase:  harness.PhaseParallel,
		Agents: []harness.AgentSpec{{Sub: ac20Sub, Name: "AC20"}},
		Resources: []harness.ResourceSpec{{
			Name:       ac20Resource,
			Containers: []string{harness.ContainerSAG},
			Database:   ac20Database,
			Agents:     []string{ac20Sub},
		}},
		Fixture: "CREATE TABLE IF NOT EXISTS t10k AS SELECT g AS id FROM generate_series(1, 10000) AS g",
		Run:     runAC20,
	})
}

type ac20Output struct {
	Code            string  `json:"code"`
	Message         string  `json:"message"`
	Rows            [][]any `json:"rows"`
	RowCount        int     `json:"row_count"`
	Truncated       bool    `json:"truncated"`
	TruncatedReason string  `json:"truncated_reason"`
}

func ac20Query(ctx context.Context, t *testing.T, session *mcp.ClientSession, extra map[string]any) (ac20Output, bool) {
	t.Helper()
	args := map[string]any{"resource": ac20Resource, "sql": ac20Select}
	for k, v := range extra {
		args[k] = v
	}
	res, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "sql_query", Arguments: args})
	if err != nil {
		t.Fatalf("sql_query %v: %v", extra, err)
	}
	if len(res.Content) != 1 {
		t.Fatalf("sql_query %v content = %+v; want one item", extra, res.Content)
	}
	tc, ok := res.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("sql_query %v content is %T", extra, res.Content[0])
	}
	var out ac20Output
	if err := json.Unmarshal([]byte(tc.Text), &out); err != nil {
		t.Fatalf("sql_query %v content %q: %v", extra, tc.Text, err)
	}
	return out, res.IsError
}

func ac20Rows(t *testing.T, out ac20Output, isError bool, want int) {
	t.Helper()
	if isError {
		t.Fatalf("sql_query = %s %q; want %d rows", out.Code, out.Message, want)
	}
	if len(out.Rows) != want || out.RowCount != want {
		t.Fatalf("sql_query rows = %d, row_count = %d; want %d", len(out.Rows), out.RowCount, want)
	}
	if !out.Truncated || out.TruncatedReason != "max_rows" {
		t.Fatalf("sql_query truncated = %v, truncated_reason = %q; want true and max_rows", out.Truncated, out.TruncatedReason)
	}
	for i, row := range out.Rows {
		if len(row) != 1 || row[0] != float64(i+1) {
			t.Fatalf("sql_query row %d = %v; want [%d]", i, row, i+1)
		}
	}
}

func runAC20(t *testing.T, env harness.Env) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	session, err := env.Client(ctx, harness.ContainerSAG, ac20Sub)
	if err != nil {
		t.Fatalf("MCP client: %v", err)
	}
	defer session.Close()

	deadline := time.Now().Add(ac20ServeWindow)
	for {
		out, isError := ac20Query(ctx, t, session, nil)
		if !isError || out.Code != "resource_unavailable" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("resource %s is not served after %v", ac20Resource, ac20ServeWindow)
		}
		time.Sleep(2 * time.Second)
	}

	t.Run("default max_rows", func(t *testing.T) {
		out, isError := ac20Query(ctx, t, session, nil)
		ac20Rows(t, out, isError, 100)
	})
	t.Run("max_rows 1000", func(t *testing.T) {
		out, isError := ac20Query(ctx, t, session, map[string]any{"max_rows": 1000})
		ac20Rows(t, out, isError, 1000)
	})
	t.Run("max_rows 1001", func(t *testing.T) {
		out, isError := ac20Query(ctx, t, session, map[string]any{"max_rows": 1001})
		if !isError || out.Code != "invalid_argument" {
			t.Fatalf("sql_query max_rows 1001 = %+v; want invalid_argument", out)
		}
	})
}
