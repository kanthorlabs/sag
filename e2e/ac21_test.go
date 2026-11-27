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
	ac21Sub         = "ac21"
	ac21Resource    = "ac21"
	ac21Database    = "sag_ac21"
	ac21MaxBytes    = 32 << 10
	ac21Unavailable = "resource_unavailable"
	ac21BigSQL      = "SELECT id, v FROM ac21_t ORDER BY id"
	ac21SmallSQL    = "SELECT id FROM ac21_t ORDER BY id LIMIT 3"
)

func init() {
	harness.Register(harness.AC{
		ID:     "AC21",
		Phase:  harness.PhaseParallel,
		Agents: []harness.AgentSpec{{Sub: ac21Sub, Name: "AC21"}},
		Resources: []harness.ResourceSpec{{
			Name:       ac21Resource,
			Containers: []string{harness.ContainerSAG},
			Database:   ac21Database,
			Agents:     []string{ac21Sub},
		}},
		Fixture: "CREATE TABLE ac21_t (id int PRIMARY KEY, v text); INSERT INTO ac21_t SELECT g, repeat('x', 1000) FROM generate_series(1, 100) g",
		Run:     runAC21,
	})
}

type ac21Output struct {
	Code            string           `json:"code"`
	Message         string           `json:"message"`
	Columns         []map[string]any `json:"columns"`
	Rows            [][]any          `json:"rows"`
	RowCount        int              `json:"row_count"`
	Truncated       bool             `json:"truncated"`
	TruncatedReason *string          `json:"truncated_reason"`
}

func ac21Served(ctx context.Context, t *testing.T, session *mcp.ClientSession, text string) (ac21Output, string) {
	t.Helper()
	deadline := time.Now().Add(70 * time.Second)
	for {
		res, err := session.CallTool(ctx, &mcp.CallToolParams{
			Name:      "sql_query",
			Arguments: map[string]any{"resource": ac21Resource, "sql": text},
		})
		if err != nil {
			t.Fatalf("sql_query %q: %v", text, err)
		}
		if len(res.Content) != 1 {
			t.Fatalf("sql_query %q content = %+v; want one item", text, res.Content)
		}
		tc, ok := res.Content[0].(*mcp.TextContent)
		if !ok {
			t.Fatalf("sql_query %q content is %T", text, res.Content[0])
		}
		var out ac21Output
		if err := json.Unmarshal([]byte(tc.Text), &out); err != nil {
			t.Fatalf("sql_query %q content %q: %v", text, tc.Text, err)
		}
		if !res.IsError {
			return out, tc.Text
		}
		if out.Code != ac21Unavailable {
			t.Fatalf("sql_query %q = %s %q; want a result", text, out.Code, out.Message)
		}
		if time.Now().After(deadline) {
			t.Fatalf("resource %s is not served after 70 s", ac21Resource)
		}
		time.Sleep(2 * time.Second)
	}
}

func ac21DataBytes(t *testing.T, text string) int {
	t.Helper()
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(text), &fields); err != nil {
		t.Fatalf("result %q: %v", text, err)
	}
	delete(fields, "duration_ms")
	delete(fields, "request_id")
	data, err := json.Marshal(fields)
	if err != nil {
		t.Fatalf("marshal result fields: %v", err)
	}
	return len(data)
}

func runAC21(t *testing.T, env harness.Env) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	session, err := env.Client(ctx, harness.ContainerSAG, ac21Sub)
	if err != nil {
		t.Fatalf("MCP client: %v", err)
	}
	defer session.Close()

	small, _ := ac21Served(ctx, t, session, ac21SmallSQL)
	if small.Truncated || small.TruncatedReason != nil || small.RowCount != 3 || len(small.Rows) != 3 {
		t.Fatalf("small result = %+v; want 3 rows and no truncation", small)
	}

	big, text := ac21Served(ctx, t, session, ac21BigSQL)
	if !big.Truncated || big.TruncatedReason == nil || *big.TruncatedReason != "max_result_bytes" {
		t.Fatalf("big result truncated = %v %v; want truncated_reason max_result_bytes", big.Truncated, big.TruncatedReason)
	}
	if big.RowCount != len(big.Rows) || big.RowCount == 0 || big.RowCount >= 100 {
		t.Fatalf("big result row_count = %d, rows = %d; want 0 < rows < 100", big.RowCount, len(big.Rows))
	}
	for i, row := range big.Rows {
		if len(row) != 2 || row[0] != float64(i+1) {
			t.Fatalf("row %d = %v; want the first rows in id order", i, row)
		}
	}
	if n := ac21DataBytes(t, text); n > ac21MaxBytes || n+1100 <= ac21MaxBytes {
		t.Fatalf("result data encoding = %d bytes; want at most %d and more than %d", n, ac21MaxBytes, ac21MaxBytes-1100)
	}
}
