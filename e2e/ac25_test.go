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
	ac25Sub         = "ac25"
	ac25Resource    = "ac25"
	ac25Database    = "sag_ac25"
	ac25Redacted    = "[REDACTED]"
	ac25Unavailable = "resource_unavailable"
	ac25SQL         = `SELECT id, api_token, note, ssn, api_token AS "API_TOKEN_COPY" FROM ac25_t ORDER BY id`
)

func init() {
	harness.Register(harness.AC{
		ID:     "AC25",
		Phase:  harness.PhaseParallel,
		Agents: []harness.AgentSpec{{Sub: ac25Sub, Name: "AC25"}},
		Resources: []harness.ResourceSpec{{
			Name:       ac25Resource,
			Containers: []string{harness.ContainerSAG},
			Database:   ac25Database,
			Agents:     []string{ac25Sub},
			Fragment:   "redact:\n  columns: [\"SSN\"]\n",
		}},
		Fixture: "CREATE TABLE ac25_t (id int PRIMARY KEY, api_token text, note text, ssn text); INSERT INTO ac25_t VALUES (1, 'tok-ac25-1', 'note-1', '111-11-1111'), (2, NULL, 'note-2', '222-22-2222')",
		Run:     runAC25,
	})
}

type ac25Output struct {
	Code    string           `json:"code"`
	Message string           `json:"message"`
	Columns []map[string]any `json:"columns"`
	Rows    [][]any          `json:"rows"`
}

func ac25Served(ctx context.Context, t *testing.T, session *mcp.ClientSession, text string) (ac25Output, string) {
	t.Helper()
	deadline := time.Now().Add(70 * time.Second)
	for {
		res, err := session.CallTool(ctx, &mcp.CallToolParams{
			Name:      "sql_query",
			Arguments: map[string]any{"resource": ac25Resource, "sql": text},
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
		var out ac25Output
		if err := json.Unmarshal([]byte(tc.Text), &out); err != nil {
			t.Fatalf("sql_query %q content %q: %v", text, tc.Text, err)
		}
		if !res.IsError {
			return out, tc.Text
		}
		if out.Code != ac25Unavailable {
			t.Fatalf("sql_query %q = %s %q; want a result", text, out.Code, out.Message)
		}
		if time.Now().After(deadline) {
			t.Fatalf("resource %s is not served after 70 s", ac25Resource)
		}
		time.Sleep(2 * time.Second)
	}
}

func runAC25(t *testing.T, env harness.Env) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	session, err := env.Client(ctx, harness.ContainerSAG, ac25Sub)
	if err != nil {
		t.Fatalf("MCP client: %v", err)
	}
	defer session.Close()

	out, text := ac25Served(ctx, t, session, ac25SQL)
	want := [][]any{
		{float64(1), ac25Redacted, "note-1", ac25Redacted, ac25Redacted},
		{float64(2), ac25Redacted, "note-2", ac25Redacted, ac25Redacted},
	}
	if len(out.Rows) != len(want) {
		t.Fatalf("rows = %v; want %v", out.Rows, want)
	}
	for i := range want {
		if len(out.Rows[i]) != len(want[i]) {
			t.Fatalf("row %d = %v; want %v", i, out.Rows[i], want[i])
		}
		for j := range want[i] {
			if out.Rows[i][j] != want[i][j] {
				t.Fatalf("row %d = %v; want %v", i, out.Rows[i], want[i])
			}
		}
	}
	if len(out.Columns) != 5 || out.Columns[1]["name"] != "api_token" || out.Columns[4]["name"] != "API_TOKEN_COPY" {
		t.Fatalf("columns = %v; want the output names unchanged", out.Columns)
	}
	for _, secret := range []string{"tok-ac25-1", "111-11-1111", "222-22-2222"} {
		if strings.Contains(text, secret) {
			t.Fatalf("result holds the redacted value %q: %s", secret, text)
		}
	}
}
