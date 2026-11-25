package mcpserver

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/auth"

	"github.com/kanthorlabs/sag/internal/sql"
)

func TestPreDispatchRejectionNotAudited(t *testing.T) {
	withCapabilities(t, sql.Capability())
	calls := &recordingCaller{}
	ts := httptest.NewServer(auth.RequireBearerToken(subAsToken, nil)(Handler(newServer(&laxInventory{}, calls, "test"))))
	t.Cleanup(ts.Close)
	post := func(body string) (int, string) {
		t.Helper()
		req, err := http.NewRequest(http.MethodPost, ts.URL, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer ac26")
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		req.Header.Set("Mcp-Protocol-Version", "2025-06-18")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		out, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		return resp.StatusCode, string(out)
	}

	cases := []struct {
		name   string
		body   string
		status int
		want   string
	}{
		{"params null", `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":null}`, http.StatusOK, `"code":-32600`},
		{"params array", `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":[]}`, http.StatusOK, `"code":-32602`},
		{"name not a string", `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":5}}`, http.StatusOK, `"code":-32602`},
		{"no params", `{"jsonrpc":"2.0","id":1,"method":"tools/call"}`, http.StatusBadRequest, "missing required"},
		{"no id", `{"jsonrpc":"2.0","method":"tools/call","params":{"name":"list_resources","arguments":{}}}`, http.StatusBadRequest, "missing id"},
		{"batch", `[{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"list_resources","arguments":{}}}]`, http.StatusBadRequest, "batching is not supported"},
	}
	for _, c := range cases {
		status, body := post(c.body)
		if status != c.status || !strings.Contains(body, c.want) {
			t.Fatalf("%s: HTTP %d %s; want HTTP %d with %q", c.name, status, body, c.status, c.want)
		}
		if n := calls.audited(); n != 0 {
			t.Fatalf("%s: records = %d; want none for a request that the JSON-RPC layer rejects", c.name, n)
		}
	}
	if status, body := post(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"list_resources","arguments":{}}}`); status != http.StatusOK || calls.audited() != 1 {
		t.Fatalf("valid tools/call: HTTP %d %s, records = %d; want HTTP 200 and one record", status, body, calls.audited())
	}
}
