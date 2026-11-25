package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/oklog/ulid/v2"

	"github.com/kanthorlabs/sag/internal/core"
	"github.com/kanthorlabs/sag/internal/pipeline"
	"github.com/kanthorlabs/sag/internal/sql"
)

type recordingCaller struct {
	mu       sync.Mutex
	requests []pipeline.Request
	rejected []pipeline.Request
	rejects  []error
	listed   []pipeline.Request
	counts   []int
	listErr  error
	result   core.Result
	err      error
	delay    time.Duration
}

func (r *recordingCaller) Execute(_ context.Context, req pipeline.Request) (core.Result, error) {
	r.mu.Lock()
	r.requests = append(r.requests, req)
	r.mu.Unlock()
	time.Sleep(r.delay)
	return r.result, r.err
}

func (r *recordingCaller) Reject(req pipeline.Request, err error) error {
	r.mu.Lock()
	r.rejected = append(r.rejected, req)
	r.rejects = append(r.rejects, err)
	r.mu.Unlock()
	return err
}

func (r *recordingCaller) Listed(req pipeline.Request, count int) error {
	r.mu.Lock()
	r.listed = append(r.listed, req)
	r.counts = append(r.counts, count)
	r.mu.Unlock()
	return r.listErr
}

func (r *recordingCaller) audited() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.requests) + len(r.rejected) + len(r.listed)
}

func (r *recordingCaller) last(t *testing.T) pipeline.Request {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.requests) == 0 {
		t.Fatal("pipeline was not called")
	}
	return r.requests[len(r.requests)-1]
}

func withCapabilities(t *testing.T, caps ...core.Capability) {
	t.Helper()
	saved := registeredCapabilities
	t.Cleanup(func() { registeredCapabilities = saved })
	registeredCapabilities = func() []core.Capability { return caps }
}

func sqlSession(t *testing.T, calls caller, sub string) *mcp.ClientSession {
	t.Helper()
	withCapabilities(t, sql.Capability())
	handler := Handler(newServer(&laxInventory{}, calls, "test"))
	if sub == "" {
		return connect(t, handler, "")
	}
	return connect(t, auth.RequireBearerToken(subAsToken, nil)(handler), sub)
}

func callTool(t *testing.T, session *mcp.ClientSession, name string, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func textContent(t *testing.T, res *mcp.CallToolResult) string {
	t.Helper()
	if len(res.Content) != 1 {
		t.Fatalf("content: got %d items, want 1", len(res.Content))
	}
	text, ok := res.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("content is %T; want text", res.Content[0])
	}
	return text.Text
}

func errorObject(t *testing.T, res *mcp.CallToolResult) map[string]string {
	t.Helper()
	if !res.IsError {
		t.Fatalf("result is not an error: %#v", res)
	}
	if res.StructuredContent != nil {
		t.Fatalf("error result has structured content: %#v", res.StructuredContent)
	}
	text := textContent(t, res)
	var obj map[string]string
	dec := json.NewDecoder(strings.NewReader(text))
	if err := dec.Decode(&obj); err != nil {
		t.Fatalf("error text is not a JSON object of strings: %q: %v", text, err)
	}
	keys := make([]string, 0, len(obj))
	for k := range obj {
		keys = append(keys, k)
	}
	if len(obj) != 3 || obj["code"] == "" || obj["message"] == "" || obj["request_id"] == "" {
		t.Fatalf("error object keys = %v; want code, message, request_id: %q", keys, text)
	}
	if _, err := ulid.ParseStrict(obj["request_id"]); err != nil {
		t.Fatalf("request_id %q is not a ULID: %v", obj["request_id"], err)
	}
	return obj
}

func TestToolsFromRegistry(t *testing.T) {
	other := core.Capability{Name: "logs", Operations: []core.Operation{{
		Name:        "search",
		Description: "Search logs.",
		InputSchema: json.RawMessage(`{"type":"object"}`),
	}}}
	withCapabilities(t, sql.Capability(), other, sql.Capability())
	session := connect(t, Handler(newServer(&laxInventory{}, &recordingCaller{}, "test")), "")
	tools, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]*mcp.Tool{}
	for _, tool := range tools.Tools {
		if _, dup := byName[tool.Name]; dup {
			t.Fatalf("tool %s listed twice", tool.Name)
		}
		byName[tool.Name] = tool
	}
	if len(byName) != 4 {
		t.Fatalf("tools = %v; want list_resources and 3 sql tools", reflect.ValueOf(byName).MapKeys())
	}
	for _, op := range sql.Capability().Operations {
		name := "sql_" + op.Name
		tool, ok := byName[name]
		if !ok {
			t.Fatalf("tool %s is missing", name)
		}
		if tool.Description != op.Description || !strings.Contains(tool.Description, untrustedResults) {
			t.Fatalf("%s description = %q", name, tool.Description)
		}
		a := tool.Annotations
		if a == nil || !a.ReadOnlyHint || !a.IdempotentHint || a.DestructiveHint == nil || *a.DestructiveHint || a.OpenWorldHint == nil || *a.OpenWorldHint {
			t.Fatalf("%s annotations: got %#v", name, a)
		}
		for label, pair := range map[string][2]any{"input": {tool.InputSchema, op.InputSchema}, "output": {tool.OutputSchema, op.OutputSchema}} {
			got, err := json.Marshal(pair[0])
			if err != nil {
				t.Fatal(err)
			}
			if !jsonEqual(t, got, pair[1].(json.RawMessage)) {
				t.Fatalf("%s %s schema = %s; want %s", name, label, got, pair[1])
			}
		}
	}

	withCapabilities(t)
	session = connect(t, Handler(newServer(&laxInventory{}, &recordingCaller{}, "test")), "")
	tools, err = session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(tools.Tools) != 1 {
		t.Fatalf("empty registry: tools = %d; want list_resources only", len(tools.Tools))
	}
}

func TestErrorResultShape(t *testing.T) {
	cases := []struct {
		name    string
		err     error
		code    string
		message string
	}{
		{"core error", &core.Error{Code: core.CodeUpstreamError, Message: "upstream error, SQLSTATE 25006", Cause: errors.New("password=hunter2 dsn")}, "upstream_error", "upstream error, SQLSTATE 25006"},
		{"not found", core.NewError(core.CodeNotFound, "resource not found"), "not_found", "resource not found"},
		{"wrapped core error", errors.Join(errors.New("hunter2"), core.NewError(core.CodeTimeout, "request timed out")), "timeout", "request timed out"},
		{"plain error", errors.New("postgres://app:hunter2@db/app"), "internal", "internal error"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			calls := &recordingCaller{err: tc.err, result: core.Result{Rows: [][]any{{"secret-value"}}}}
			res := callTool(t, sqlSession(t, calls, "ac05"), "sql_query", map[string]any{"resource": "main", "sql": "SELECT 1"})
			obj := errorObject(t, res)
			if obj["code"] != tc.code || obj["message"] != tc.message {
				t.Fatalf("error object = %v; want code %s, message %q", obj, tc.code, tc.message)
			}
			text := textContent(t, res)
			if strings.Contains(text, "hunter2") || strings.Contains(text, "secret-value") {
				t.Fatalf("error text leaks a cause or a value: %q", text)
			}
			if got := calls.last(t).ID; got != obj["request_id"] {
				t.Fatalf("request_id %q differs from pipeline request ID %q", obj["request_id"], got)
			}
		})
	}

	calls := &recordingCaller{}
	res := callTool(t, sqlSession(t, calls, "ac05"), "sql_query", map[string]any{"resource": 7, "sql": "SELECT 1"})
	if obj := errorObject(t, res); obj["code"] != "invalid_argument" {
		t.Fatalf("bad resource type: error object = %v; want invalid_argument", obj)
	}
	if len(calls.requests) != 0 {
		t.Fatal("pipeline called with invalid arguments")
	}
	if len(calls.rejected) != 1 || calls.rejected[0].ID != errorObject(t, res)["request_id"] || calls.rejected[0].Sub != "ac05" || calls.rejected[0].Operation != "query" {
		t.Fatalf("rejected = %#v; want one audited rejection with the response request_id", calls.rejected)
	}
}

func TestRequestReachesPipeline(t *testing.T) {
	calls := &recordingCaller{}
	session := sqlSession(t, calls, "ac05")
	args := map[string]any{"resource": "main", "schema": "app", "table": "orders"}
	callTool(t, session, "sql_describe_table", args)
	got := calls.last(t)
	if got.Sub != "ac05" || got.JTI != "jti-ac05" || got.RemoteAddr == "" || got.Resource != "main" || got.Capability != "sql" || got.Operation != "describe_table" {
		t.Fatalf("pipeline request = %#v", got)
	}
	want, _ := json.Marshal(args)
	if !jsonEqual(t, got.Input, want) {
		t.Fatalf("input = %s; want %s", got.Input, want)
	}
	if _, err := ulid.ParseStrict(got.ID); err != nil {
		t.Fatalf("request ID %q is not a ULID", got.ID)
	}

	callTool(t, session, "sql_list_tables", map[string]any{"resource": "main"})
	callTool(t, session, "sql_list_tables", map[string]any{"resource": "main"})
	calls.mu.Lock()
	ids := []string{calls.requests[1].ID, calls.requests[2].ID}
	ops := []string{calls.requests[1].Operation, calls.requests[2].Operation}
	calls.mu.Unlock()
	if ids[0] == ids[1] || ops[0] != "list_tables" || ops[1] != "list_tables" {
		t.Fatalf("ids = %v, operations = %v; want unique IDs and list_tables", ids, ops)
	}

	callTool(t, sqlSession(t, calls, ""), "sql_query", map[string]any{"resource": "main", "sql": "SELECT 1"})
	if got := calls.last(t); got.Sub != "" {
		t.Fatalf("no identity: sub = %q; want empty", got.Sub)
	}
}

func TestSuccessResultShape(t *testing.T) {
	calls := &recordingCaller{
		delay: 20 * time.Millisecond,
		result: core.Result{
			Columns:         []core.Column{{Name: "id", Type: "int8"}, {Name: "name", Type: "text"}},
			Rows:            [][]any{{int64(1), "a"}, {int64(2), nil}},
			Truncated:       true,
			TruncatedReason: core.TruncatedMaxRows,
			Trace:           core.Trace{Input: "SELECT id, name FROM t", InputSHA256: "abc"},
		},
	}
	res := callTool(t, sqlSession(t, calls, "ac05"), "sql_query", map[string]any{"resource": "main", "sql": "SELECT id, name FROM t"})
	if res.IsError {
		t.Fatalf("result is an error: %q", textContent(t, res))
	}
	structured, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	if !jsonEqual(t, []byte(textContent(t, res)), structured) {
		t.Fatalf("text content differs from structured content %s", structured)
	}
	var out map[string]any
	if err := json.Unmarshal(structured, &out); err != nil {
		t.Fatal(err)
	}
	wantColumns := []any{map[string]any{"name": "id", "type": "int8"}, map[string]any{"name": "name", "type": "text"}}
	wantRows := []any{[]any{float64(1), "a"}, []any{float64(2), nil}}
	if !reflect.DeepEqual(out["columns"], wantColumns) || !reflect.DeepEqual(out["rows"], wantRows) {
		t.Fatalf("columns, rows = %v, %v", out["columns"], out["rows"])
	}
	if out["row_count"] != float64(2) || out["truncated"] != true || out["truncated_reason"] != "max_rows" {
		t.Fatalf("output = %s", structured)
	}
	if d, ok := out["duration_ms"].(float64); !ok || d < 20 {
		t.Fatalf("duration_ms = %v; want at least 20", out["duration_ms"])
	}
	if out["request_id"] != calls.last(t).ID {
		t.Fatalf("request_id = %v; want %s", out["request_id"], calls.last(t).ID)
	}
	if len(out) != 7 {
		t.Fatalf("output keys = %d; want 7 (no trace): %s", len(out), structured)
	}

	calls.result = core.Result{}
	calls.delay = 0
	res = callTool(t, sqlSession(t, calls, "ac05"), "sql_list_tables", map[string]any{"resource": "main"})
	text := textContent(t, res)
	if !strings.Contains(text, `"columns":[]`) || !strings.Contains(text, `"rows":[]`) || !strings.Contains(text, `"row_count":0`) || !strings.Contains(text, `"truncated":false`) || strings.Contains(text, "truncated_reason") || strings.Contains(text, "duration_ms") {
		t.Fatalf("empty result text = %s", text)
	}
}

type panicCaller struct {
	recordingCaller
}

func (p *panicCaller) Execute(context.Context, pipeline.Request) (core.Result, error) {
	panic("index out of range")
}

func TestToolPanicReturnsInternal(t *testing.T) {
	calls := &panicCaller{}
	session := sqlSession(t, calls, "")
	obj := errorObject(t, callTool(t, session, "sql_query", map[string]any{"resource": "main", "sql": "SELECT 1"}))
	if obj["code"] != string(core.CodeInternal) || obj["message"] != "internal error" {
		t.Fatalf("error = %v; want internal", obj)
	}
	obj = errorObject(t, callTool(t, session, "sql_query", map[string]any{"resource": "main", "sql": "SELECT 1"}))
	if obj["code"] != string(core.CodeInternal) {
		t.Fatalf("second call error = %v; want internal from a live server", obj)
	}
	calls.mu.Lock()
	defer calls.mu.Unlock()
	if len(calls.rejected) != 2 || calls.rejected[0].Operation != "query" || calls.rejected[0].Resource != "main" {
		t.Fatalf("rejected = %+v; want one record for each panic", calls.rejected)
	}
	assertCode(t, calls.rejects[0], core.CodeInternal)
}
