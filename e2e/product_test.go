//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kanthorlabs/sag/e2e/harness"
)

const (
	productSub      = "product"
	productResource = "product"
	productDatabase = "sag_product"
	productProbe    = "sag_product_probe_7f3"
	productSelect   = "select  1  as  " + productProbe
	productRendered = "DECLARE sag_c NO SCROLL CURSOR FOR SELECT 1 AS " + productProbe
	productInsert   = "INSERT INTO product_t VALUES (7317)"
	productSQLLimit = 16 * 1024
)

func init() {
	harness.Register(harness.AC{
		ID:     "Product_RejectsAllUntilC1",
		Phase:  harness.PhaseParallel,
		Agents: []harness.AgentSpec{{Sub: productSub, Name: "Product"}},
		Resources: []harness.ResourceSpec{{
			Name:       productResource,
			Containers: []string{harness.ContainerSAG},
			Database:   productDatabase,
			Agents:     []string{productSub},
		}},
		Fixture: "CREATE TABLE IF NOT EXISTS product_t (id int)",
		Run:     runProductRejectsAll,
	})
	harness.Register(harness.AC{
		ID:    "Product_SQLSizeLimit",
		Phase: harness.PhaseParallel,
		Run:   runProductSQLSizeLimit,
	})
}

type productOutput struct {
	Code    string  `json:"code"`
	Message string  `json:"message"`
	Rows    [][]any `json:"rows"`
}

func productCall(ctx context.Context, t *testing.T, env harness.Env, text string) (productOutput, bool) {
	t.Helper()
	session, err := env.Client(ctx, harness.ContainerSAG, productSub)
	if err != nil {
		t.Fatalf("MCP client: %v", err)
	}
	defer session.Close()
	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "sql_query",
		Arguments: map[string]any{"resource": productResource, "sql": text},
	})
	if err != nil {
		t.Fatalf("sql_query of %d bytes: %v", len(text), err)
	}
	if len(res.Content) != 1 {
		t.Fatalf("sql_query of %d bytes content = %+v; want one item", len(text), res.Content)
	}
	tc, ok := res.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("sql_query error content is %T", res.Content[0])
	}
	var out productOutput
	if err := json.Unmarshal([]byte(tc.Text), &out); err != nil {
		t.Fatalf("sql_query content %q: %v", tc.Text, err)
	}
	return out, res.IsError
}

func productQuery(ctx context.Context, t *testing.T, env harness.Env, text string) productOutput {
	t.Helper()
	out, isError := productCall(ctx, t, env, text)
	if !isError {
		t.Fatalf("sql_query of %d bytes returned a result; want an error: %+v", len(text), out)
	}
	return out
}

func productSelectRow(ctx context.Context, t *testing.T, env harness.Env) {
	t.Helper()
	out, isError := productCall(ctx, t, env, productSelect)
	if isError {
		t.Fatalf("sql_query %q = %s %q; want a result", productSelect, out.Code, out.Message)
	}
	if len(out.Rows) != 1 || len(out.Rows[0]) != 1 || out.Rows[0][0] != float64(1) {
		t.Fatalf("sql_query %q rows = %v; want [[1]]", productSelect, out.Rows)
	}
}

func productServed(ctx context.Context, t *testing.T, env harness.Env) {
	t.Helper()
	deadline := time.Now().Add(70 * time.Second)
	for {
		out, isError := productCall(ctx, t, env, productSelect)
		if !isError || out.Code != "resource_unavailable" {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("resource %s is not served after 70 s", productResource)
		}
		time.Sleep(2 * time.Second)
	}
}

func runProductRejectsAll(t *testing.T, env harness.Env) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	productServed(ctx, t, env)
	start := time.Now()
	t.Run("SELECT served by C1", func(t *testing.T) {
		productSelectRow(ctx, t, env)
	})
	t.Run("INSERT rejected by C1", func(t *testing.T) {
		if got := productQuery(ctx, t, env, productInsert); got.Code != "query_rejected" || got.Message != "statement rejected: statement:InsertStmt" {
			t.Fatalf("sql_query %q = %+v; want query_rejected \"statement rejected: statement:InsertStmt\"", productInsert, got)
		}
	})
	pg := env.Postgres(env.Major())
	var logs string
	deadline := time.Now().Add(30 * time.Second)
	for {
		var err error
		logs, err = pg.Logs(ctx, start.Add(-time.Second))
		if err != nil {
			t.Fatalf("PostgreSQL logs: %v", err)
		}
		if strings.Contains(logs, productRendered) || time.Now().After(deadline) {
			break
		}
		time.Sleep(time.Second)
	}
	if !strings.Contains(logs, productRendered) {
		t.Fatalf("PostgreSQL log has no statement %q:\n%s", productRendered, logs)
	}
	if n, rendered := strings.Count(logs, productProbe), strings.Count(logs, productRendered); n != rendered {
		t.Fatalf("PostgreSQL log holds %q %d times, but only %d times in the rendered form %q:\n%s", productProbe, n, rendered, productRendered, logs)
	}
	if strings.Contains(logs, "VALUES (7317)") {
		t.Fatalf("PostgreSQL log holds %q; the product sent a rejected INSERT to the server:\n%s", "VALUES (7317)", logs)
	}
}

func runProductSQLSizeLimit(t *testing.T, env harness.Env) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	productServed(ctx, t, env)
	chain := func(n int) string {
		return "SELECT 1" + strings.Repeat("+1", n)
	}
	atLimit := chain((productSQLLimit - len("SELECT 1")) / 2)
	if len(atLimit) != productSQLLimit {
		t.Fatalf("chain length = %d; want %d", len(atLimit), productSQLLimit)
	}
	if got, isError := productCall(ctx, t, env, atLimit); isError && got.Code == "invalid_argument" {
		t.Fatalf("sql_query at %d bytes = %+v; want no invalid_argument", len(atLimit), got)
	}
	for _, text := range []string{atLimit + "1", chain(22000)} {
		if got := productQuery(ctx, t, env, text); got.Code != "invalid_argument" || got.Message != "sql exceeds 16384 bytes" {
			t.Fatalf("sql_query at %d bytes = %+v; want invalid_argument", len(text), got)
		}
	}
	resp, err := env.HTTPClient().Get(env.URL(harness.ContainerSAG) + "/healthz")
	if err != nil {
		t.Fatalf("GET /healthz after the size limit calls: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /healthz = %d after the size limit calls", resp.StatusCode)
	}
	productSelectRow(ctx, t, env)
}
