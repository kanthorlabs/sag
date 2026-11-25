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
	productSelect   = "SELECT 1 AS sag_product_probe_7f3"
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

type productError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func productQuery(ctx context.Context, t *testing.T, env harness.Env, text string) productError {
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
	if !res.IsError || len(res.Content) != 1 {
		t.Fatalf("sql_query of %d bytes returned a result; want an error: %+v", len(text), res.Content)
	}
	tc, ok := res.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("sql_query error content is %T", res.Content[0])
	}
	var out productError
	if err := json.Unmarshal([]byte(tc.Text), &out); err != nil {
		t.Fatalf("sql_query error %q: %v", tc.Text, err)
	}
	return out
}

func productServed(ctx context.Context, t *testing.T, env harness.Env) {
	t.Helper()
	deadline := time.Now().Add(70 * time.Second)
	for productQuery(ctx, t, env, productSelect).Code == "resource_unavailable" {
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
	t.Run("SELECT rejected until layer 3", func(t *testing.T) {
		if got := productQuery(ctx, t, env, productSelect); got.Code != "query_rejected" {
			t.Fatalf("sql_query %q = %+v; want query_rejected", productSelect, got)
		}
	})
	t.Run("INSERT rejected by C1", func(t *testing.T) {
		if got := productQuery(ctx, t, env, productInsert); got.Code != "query_rejected" {
			t.Fatalf("sql_query %q = %+v; want query_rejected", productInsert, got)
		}
	})
	logs, err := env.Postgres(env.Major()).Logs(ctx, start.Add(-time.Second))
	if err != nil {
		t.Fatalf("PostgreSQL logs: %v", err)
	}
	for _, text := range []string{"sag_product_probe_7f3", "VALUES (7317)"} {
		if strings.Contains(logs, text) {
			t.Fatalf("PostgreSQL log holds %q; the product sent agent text to the server:\n%s", text, logs)
		}
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
	if got := productQuery(ctx, t, env, atLimit); got.Code != "query_rejected" {
		t.Fatalf("sql_query at %d bytes = %+v; want query_rejected", len(atLimit), got)
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
	if got := productQuery(ctx, t, env, productSelect); got.Code != "query_rejected" {
		t.Fatalf("sql_query after the size limit calls = %+v; want query_rejected from a live daemon", got)
	}
}
