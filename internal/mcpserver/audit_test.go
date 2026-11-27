package mcpserver

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kanthorlabs/sag/internal/core"
	"github.com/kanthorlabs/sag/internal/inventory"
	"github.com/kanthorlabs/sag/internal/pipeline"
	"github.com/kanthorlabs/sag/internal/sql"
)

func assertCode(t *testing.T, err error, code core.Code) {
	t.Helper()
	var ce *core.Error
	if !errors.As(err, &ce) || ce.Code != code {
		t.Fatalf("error = %v; want code %s", err, code)
	}
}

func auditSession(t *testing.T, inv visibility, calls caller, sub string) *mcp.ClientSession {
	t.Helper()
	withCapabilities(t, sql.Capability())
	return connect(t, auth.RequireBearerToken(subAsToken, nil)(Handler(newServer(inv, calls, "test"))), sub)
}

func servedEntry(name string, agents ...string) inventory.Entry {
	return inventory.Entry{Name: name, Kind: "postgres", Capabilities: []string{"sql"}, Agents: agents, Status: inventory.StatusServed}
}

func TestInitializeAndToolsListNotAudited(t *testing.T) {
	calls := &recordingCaller{}
	session := auditSession(t, &laxInventory{}, calls, "ac26")
	if _, err := session.ListTools(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	if n := calls.audited(); n != 0 {
		t.Fatalf("records = %d; want none for initialize and tools/list", n)
	}
}

func TestListResourcesAudited(t *testing.T) {
	calls := &recordingCaller{}
	inv := &laxInventory{entries: []inventory.Entry{servedEntry("a", "ac26"), servedEntry("b", "ac26"), servedEntry("c", "other")}}
	session := auditSession(t, inv, calls, "ac26")
	if got := callListResources(t, session); len(got.Resources) != 2 {
		t.Fatalf("resources = %+v", got.Resources)
	}
	if n := calls.audited(); n != 1 {
		t.Fatalf("records = %d; want 1", n)
	}
	got := calls.listed[0]
	if got.Sub != "ac26" || got.JTI != "jti-ac26" || got.RemoteAddr == "" || got.Operation != "list_resources" || got.Resource != "" || got.Capability != "" || got.ID == "" || calls.counts[0] != 2 {
		t.Fatalf("listed request = %+v, count = %d", got, calls.counts[0])
	}
}

func TestListResourcesAuditFailureReturnsInternal(t *testing.T) {
	calls := &recordingCaller{listErr: core.NewError(core.CodeInternal, "internal error")}
	inv := &laxInventory{entries: []inventory.Entry{servedEntry("a", "ac27")}}
	res := callTool(t, auditSession(t, inv, calls, "ac27"), "list_resources", map[string]any{})
	if obj := errorObject(t, res); obj["code"] != string(core.CodeInternal) {
		t.Fatalf("error = %v; want internal", obj)
	}
}

func TestListResourcesBadArgumentsAudited(t *testing.T) {
	calls := &recordingCaller{}
	res := callTool(t, auditSession(t, &laxInventory{}, calls, "ac26"), "list_resources", map[string]any{"x": 1})
	if obj := errorObject(t, res); obj["code"] != string(core.CodeInvalidArgument) {
		t.Fatalf("error = %v; want invalid_argument", obj)
	}
	if n := calls.audited(); n != 1 || calls.rejected[0].Operation != "list_resources" {
		t.Fatalf("records = %d, rejected = %+v; want one reject", n, calls.rejected)
	}
	assertCode(t, calls.rejects[0], core.CodeInvalidArgument)
}

type panicInventory struct{}

func (panicInventory) Visible(string) []inventory.Entry {
	panic("inventory broke")
}

func TestListResourcesPanicAudited(t *testing.T) {
	calls := &recordingCaller{}
	res := callTool(t, auditSession(t, panicInventory{}, calls, "ac26"), "list_resources", map[string]any{})
	if obj := errorObject(t, res); obj["code"] != string(core.CodeInternal) {
		t.Fatalf("error = %v; want internal", obj)
	}
	if n := calls.audited(); n != 1 || len(calls.rejected) != 1 {
		t.Fatalf("records = %d; want one reject", n)
	}
	assertCode(t, calls.rejects[0], core.CodeInternal)
}

func TestUnknownToolAudited(t *testing.T) {
	calls := &recordingCaller{}
	session := auditSession(t, &laxInventory{}, calls, "ac26")
	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "drop_everything", Arguments: map[string]any{"resource": "main"}})
	if err != nil {
		t.Fatalf("unknown tool: want an error result, got %v", err)
	}
	obj := errorObject(t, res)
	if obj["code"] != string(core.CodeInvalidArgument) || obj["message"] != "unknown tool" {
		t.Fatalf("unknown tool error = %v; want invalid_argument, unknown tool", obj)
	}
	if strings.Contains(textContent(t, res), "drop_everything") {
		t.Fatalf("error text echoes the tool name: %s", textContent(t, res))
	}
	if n := calls.audited(); n != 1 || len(calls.rejected) != 1 {
		t.Fatalf("records = %d; want one reject", n)
	}
	got := calls.rejected[0]
	if got.Sub != "ac26" || got.JTI != "jti-ac26" || got.Operation != "drop_everything" || got.ID != obj["request_id"] {
		t.Fatalf("rejected request = %+v", got)
	}
	assertCode(t, calls.rejects[0], core.CodeInvalidArgument)
}

func TestKnownToolsAuditedOnce(t *testing.T) {
	calls := &recordingCaller{}
	session := auditSession(t, &laxInventory{}, calls, "ac26")
	callTool(t, session, "sql_query", map[string]any{"resource": "main", "sql": "SELECT 1"})
	callListResources(t, session)
	if n := calls.audited(); n != 2 || len(calls.rejected) != 0 {
		t.Fatalf("records = %d, rejected = %d; want 2 and 0", n, len(calls.rejected))
	}
}

var _ caller = (*pipeline.Pipeline)(nil)
