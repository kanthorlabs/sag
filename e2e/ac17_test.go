//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kanthorlabs/sag/e2e/harness"
)

const (
	ac17Sub         = "ac17"
	ac17Resource    = "ac17"
	ac17Database    = "sag_ac17"
	ac17Unavailable = "resource_unavailable"
	ac17Declare     = "DECLARE sag_c NO SCROLL CURSOR FOR "
)

var ac17Statement = regexp.MustCompile(`(?m)LOG:\s+(?:statement|execute [^:]*):\s?(.*)$`)

type ac17Case struct {
	text     string
	rendered string
	absent   []string
}

func ac17Cases() []ac17Case {
	return []ac17Case{
		{"select   1", "SELECT 1", []string{"select   1"}},
		{"select  'ac17_probe'  /* ac17_comment */  as   ac17_col", "SELECT 'ac17_probe' AS ac17_col", []string{"ac17_comment", "select  'ac17_probe'"}},
	}
}

func init() {
	harness.Register(harness.AC{
		ID:     "AC17",
		Phase:  harness.PhaseParallel,
		Agents: []harness.AgentSpec{{Sub: ac17Sub, Name: "AC17"}},
		Resources: []harness.ResourceSpec{{
			Name:       ac17Resource,
			Containers: []string{harness.ContainerSAG},
			Database:   ac17Database,
			Agents:     []string{ac17Sub},
		}},
		Run: runAC17,
	})
}

type ac17Output struct {
	Code    string  `json:"code"`
	Message string  `json:"message"`
	Rows    [][]any `json:"rows"`
}

func ac17Served(ctx context.Context, t *testing.T, session *mcp.ClientSession, text string) ac17Output {
	t.Helper()
	deadline := time.Now().Add(70 * time.Second)
	for {
		res, err := session.CallTool(ctx, &mcp.CallToolParams{
			Name:      "sql_query",
			Arguments: map[string]any{"resource": ac17Resource, "sql": text},
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
		var out ac17Output
		if err := json.Unmarshal([]byte(tc.Text), &out); err != nil {
			t.Fatalf("sql_query %q content %q: %v", text, tc.Text, err)
		}
		if !res.IsError {
			return out
		}
		if out.Code != ac17Unavailable {
			t.Fatalf("sql_query %q = %s %q; want a result", text, out.Code, out.Message)
		}
		if time.Now().After(deadline) {
			t.Fatalf("resource %s is not served after 70 s", ac17Resource)
		}
		time.Sleep(2 * time.Second)
	}
}

func ac17LoggedStatements(logs string) []string {
	var out []string
	for _, m := range ac17Statement.FindAllStringSubmatch(logs, -1) {
		out = append(out, strings.TrimRight(m[1], "\r"))
	}
	return out
}

func runAC17(t *testing.T, env harness.Env) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	session, err := env.Client(ctx, harness.ContainerSAG, ac17Sub)
	if err != nil {
		t.Fatalf("MCP client: %v", err)
	}
	defer session.Close()

	start := time.Now()
	cases := ac17Cases()
	for _, c := range cases {
		if out := ac17Served(ctx, t, session, c.text); len(out.Rows) != 1 {
			t.Fatalf("sql_query %q rows = %v; want one row", c.text, out.Rows)
		}
	}

	pg := env.Postgres(env.Major())
	var logs string
	var statements []string
	deadline := time.Now().Add(30 * time.Second)
	for {
		logs, err = pg.Logs(ctx, start.Add(-time.Second))
		if err != nil {
			t.Fatalf("PostgreSQL logs: %v", err)
		}
		statements = ac17LoggedStatements(logs)
		if !slices.ContainsFunc(cases, func(c ac17Case) bool { return !slices.Contains(statements, ac17Declare+c.rendered) }) || time.Now().After(deadline) {
			break
		}
		time.Sleep(time.Second)
	}
	for _, c := range cases {
		if !slices.Contains(statements, ac17Declare+c.rendered) {
			t.Errorf("PostgreSQL log has no statement %q for the request %q", ac17Declare+c.rendered, c.text)
		}
		for _, raw := range c.absent {
			if strings.Contains(logs, raw) {
				t.Errorf("PostgreSQL log holds %q from the request %q; SAG sent text that it did not render", raw, c.text)
			}
		}
	}
}
