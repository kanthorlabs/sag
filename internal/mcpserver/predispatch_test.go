package mcpserver

import (
	"fmt"
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
	post := func(body string, headers ...string) (int, string) {
		t.Helper()
		req, err := http.NewRequest(http.MethodPost, ts.URL, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer ac26")
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		req.Header.Set("Mcp-Protocol-Version", "2025-06-18")
		for i := 0; i+1 < len(headers); i += 2 {
			if headers[i+1] == "" {
				req.Header.Del(headers[i])
				continue
			}
			req.Header.Set(headers[i], headers[i+1])
		}
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
		name    string
		body    string
		status  int
		want    string
		headers []string
	}{
		{"params null", `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":null}`, http.StatusOK, `"code":-32600`, nil},
		{"params array", `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":[]}`, http.StatusOK, `"code":-32602`, nil},
		{"name not a string", `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":5}}`, http.StatusOK, `"code":-32602`, nil},
		{"no params", `{"jsonrpc":"2.0","id":1,"method":"tools/call"}`, http.StatusBadRequest, "missing required", nil},
		{"no id", `{"jsonrpc":"2.0","method":"tools/call","params":{"name":"list_resources","arguments":{}}}`, http.StatusBadRequest, "missing id", nil},
		{"batch", `[{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"list_resources","arguments":{}}}]`, http.StatusBadRequest, "batching is not supported", nil},
		{"prompts/get", `{"jsonrpc":"2.0","id":1,"method":"prompts/get","params":{"name":"agent-prompt"}}`, http.StatusOK, `"code":-32602,"message":"unknown prompt \"agent-prompt\""`, nil},
		{"resources/read", `{"jsonrpc":"2.0","id":1,"method":"resources/read","params":{"uri":"file:///agent-uri"}}`, http.StatusOK, `"data":{"uri":"file:///agent-uri"}`, nil},
		{"unknown method", `{"jsonrpc":"2.0","id":1,"method":"agent/method","params":{}}`, http.StatusBadRequest, `"agent/method" unsupported`, nil},
		{"unknown method new", `{"jsonrpc":"2.0","id":1,"method":"agent/method","params":{` + metaNew + `}}`, http.StatusNotFound, `"code":-32601`, []string{"Mcp-Protocol-Version", protocolNew, "Mcp-Method", "agent/method"}},
		{"Mcp-Name differs", `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"list_resources","arguments":{},` + metaNew + `}}`, http.StatusBadRequest, `"code":-32020,"message":"header mismatch: Mcp-Name header value 'agent-name'`, []string{"Mcp-Protocol-Version", protocolNew, "Mcp-Method", "tools/call", "Mcp-Name", "agent-name"}},
		{"unsupported version", `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"list_resources","arguments":{},"_meta":{"io.modelcontextprotocol/protocolVersion":"9999-01-01","io.modelcontextprotocol/clientCapabilities":{},"io.modelcontextprotocol/clientInfo":{"name":"test","version":"1"}}}}`, http.StatusBadRequest, `"requested":"9999-01-01"`, []string{"Mcp-Protocol-Version", "9999-01-01", "Mcp-Method", "tools/call", "Mcp-Name", "list_resources"}},
	}
	for _, c := range cases {
		status, body := post(c.body, c.headers...)
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
	call := `{"jsonrpc":"2.0","id":%d,"method":"tools/call","params":{"name":"list_resources","arguments":{}}}`
	batch := "[" + fmt.Sprintf(call, 1) + "," + fmt.Sprintf(call, 2) + "," + fmt.Sprintf(call, 3) + "]"
	for _, version := range []string{"", "2025-03-26"} {
		before := calls.audited()
		status, body := post(batch, "Mcp-Protocol-Version", version)
		if status != http.StatusOK || strings.Count(body, `"result"`) != 3 || calls.audited()-before != 3 {
			t.Fatalf("batch of 3 with version %q: HTTP %d %s, records = %d; want 3 results and one record for each call", version, status, body, calls.audited()-before)
		}
	}
}
