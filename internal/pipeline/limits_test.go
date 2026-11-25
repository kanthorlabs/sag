package pipeline

import (
	"encoding/json"
	"testing"
	"time"

	_ "github.com/kanthorlabs/sag/internal/adapter/postgres"
	"github.com/kanthorlabs/sag/internal/config"
	"github.com/kanthorlabs/sag/internal/core"
	"github.com/kanthorlabs/sag/internal/inventory"
)

func limitsEntry(lim core.Limits) inventory.Entry {
	return inventory.Entry{Name: "main", Kind: "postgres", Capabilities: []string{"sql"}, Limits: lim}
}

func limitsRequest(operation, input string) Request {
	return Request{Resource: "main", Capability: "sql", Operation: operation, Input: json.RawMessage(input)}
}

func TestRequestValuesOverrideResourceLimits(t *testing.T) {
	got, err := resolveLimits(limitsEntry(testLimits), limitsRequest("query", `{"resource":"main","sql":"SELECT 1","max_rows":1000,"max_result_bytes":2097152,"timeout_seconds":30}`))
	if err != nil {
		t.Fatal(err)
	}
	want := core.Limits{MaxRows: 1000, MaxResultBytes: 2 << 20, MaxCellBytes: testLimits.MaxCellBytes, Timeout: 30 * time.Second, IdleTimeout: 35 * time.Second}
	if got != want {
		t.Fatalf("limits = %+v; want %+v", got, want)
	}
}

func TestNoRequestValuesKeepResourceLimits(t *testing.T) {
	got, err := resolveLimits(limitsEntry(testLimits), limitsRequest("query", `{"resource":"main","sql":"SELECT 1"}`))
	if err != nil {
		t.Fatal(err)
	}
	if got != testLimits {
		t.Fatalf("limits = %+v; want %+v", got, testLimits)
	}
}

func TestRequestAboveHardMaxInvalidArgument(t *testing.T) {
	hard := config.HardMaximums()
	cases := map[string]string{
		"max_rows above":         `{"resource":"main","sql":"SELECT 1","max_rows":1001}`,
		"max_result_bytes above": `{"resource":"main","sql":"SELECT 1","max_result_bytes":2097153}`,
		"timeout_seconds above":  `{"resource":"main","sql":"SELECT 1","timeout_seconds":31}`,
		"max_rows zero":          `{"resource":"main","sql":"SELECT 1","max_rows":0}`,
		"timeout_seconds below":  `{"resource":"main","sql":"SELECT 1","timeout_seconds":-1}`,
		"max_rows fraction":      `{"resource":"main","sql":"SELECT 1","max_rows":1.5}`,
		"max_rows string":        `{"resource":"main","sql":"SELECT 1","max_rows":"10"}`,
		"max_rows huge":          `{"resource":"main","sql":"SELECT 1","max_rows":1e30}`,
	}
	for name, input := range cases {
		_, err := resolveLimits(limitsEntry(testLimits), limitsRequest("query", input))
		if err == nil {
			t.Fatalf("%s: no error", name)
		}
		assertCode(t, err, core.CodeInvalidArgument)
	}
	edge := `{"resource":"main","sql":"SELECT 1","max_rows":1000,"max_result_bytes":2097152,"timeout_seconds":30}`
	got, err := resolveLimits(limitsEntry(testLimits), limitsRequest("query", edge))
	if err != nil {
		t.Fatal(err)
	}
	if got.MaxRows != hard.MaxRows || got.MaxResultBytes != hard.MaxResultBytes || got.Timeout != hard.StatementTimeout {
		t.Fatalf("limits at the hard maximum = %+v", got)
	}
}

func TestUnknownKeyInvalidArgument(t *testing.T) {
	cases := map[string]struct{ operation, input string }{
		"query extra key":          {"query", `{"resource":"main","sql":"SELECT 1","max_cell_bytes":10}`},
		"query timeout key":        {"query", `{"resource":"main","sql":"SELECT 1","timeout":10}`},
		"list_tables limit key":    {"list_tables", `{"resource":"main","max_rows":10}`},
		"describe_table limit":     {"describe_table", `{"resource":"main","table":"t","timeout_seconds":5}`},
		"not an object":            {"query", `["main"]`},
		"unknown operation":        {"drop", `{"resource":"main"}`},
		"key with NUL":             {"query", `{"resource":"main","sql":"SELECT 1","s\u0000ql":"x"}`},
		"value with NUL":           {"query", `{"resource":"main","sql":"SELECT 1\u0000; DROP TABLE t"}`},
		"resource with NUL":        {"query", `{"resource":"main\u0000","sql":"SELECT 1"}`},
		"nested value with NUL":    {"query", `{"resource":"main","sql":["\u0000"]}`},
		"list_tables schema NUL":   {"list_tables", `{"resource":"main","schema":"pub\u0000lic"}`},
		"describe_table table NUL": {"describe_table", `{"resource":"main","table":"t\u0000"}`},
	}
	for name, c := range cases {
		_, err := resolveLimits(limitsEntry(testLimits), limitsRequest(c.operation, c.input))
		if err == nil {
			t.Fatalf("%s: no error", name)
		}
		assertCode(t, err, core.CodeInvalidArgument)
	}
	for _, c := range []struct{ operation, input string }{
		{"list_tables", `{"resource":"main","schema":"public"}`},
		{"describe_table", `{"resource":"main","schema":"public","table":"t"}`},
	} {
		if _, err := resolveLimits(limitsEntry(testLimits), limitsRequest(c.operation, c.input)); err != nil {
			t.Fatalf("%s: %v", c.operation, err)
		}
	}
}

func TestIdleTimeoutIsStatementPlus5s(t *testing.T) {
	base := core.Limits{MaxRows: 100, MaxResultBytes: 32 << 10, MaxCellBytes: 4 << 10, Timeout: 15 * time.Second, IdleTimeout: 20 * time.Second}
	cases := []struct {
		name     string
		idle     time.Duration
		input    string
		wantStmt time.Duration
		wantIdle time.Duration
	}{
		{"resource default", 20 * time.Second, `{"resource":"main","sql":"SELECT 1"}`, 15 * time.Second, 20 * time.Second},
		{"request raises timeout", 20 * time.Second, `{"resource":"main","sql":"SELECT 1","timeout_seconds":25}`, 25 * time.Second, 30 * time.Second},
		{"request lowers timeout keeps resource idle", 20 * time.Second, `{"resource":"main","sql":"SELECT 1","timeout_seconds":1}`, time.Second, 20 * time.Second},
		{"capped at hard maximum", 20 * time.Second, `{"resource":"main","sql":"SELECT 1","timeout_seconds":30}`, 30 * time.Second, 35 * time.Second},
		{"resource idle above statement", 33 * time.Second, `{"resource":"main","sql":"SELECT 1","timeout_seconds":2}`, 2 * time.Second, 33 * time.Second},
	}
	for _, c := range cases {
		lim := base
		lim.IdleTimeout = c.idle
		got, err := resolveLimits(limitsEntry(lim), limitsRequest("query", c.input))
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if got.Timeout != c.wantStmt || got.IdleTimeout != c.wantIdle {
			t.Fatalf("%s: timeout %s idle %s; want %s and %s", c.name, got.Timeout, got.IdleTimeout, c.wantStmt, c.wantIdle)
		}
	}
}
