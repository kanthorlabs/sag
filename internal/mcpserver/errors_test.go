package mcpserver

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/auth"

	"github.com/kanthorlabs/sag/internal/core"
	"github.com/kanthorlabs/sag/internal/inventory"
	"github.com/kanthorlabs/sag/internal/pipeline"
	"github.com/kanthorlabs/sag/internal/sql"
)

func TestErrorCodesClosedSet(t *testing.T) {
	prd := []core.Code{"unauthenticated", "not_found", "invalid_argument", "query_rejected", "timeout", "rate_limited", "resource_unavailable", "upstream_error", "internal"}
	if !slices.Equal(errorCodes, prd) {
		t.Fatalf("error codes = %v; want %v", errorCodes, prd)
	}
	decode := func(t *testing.T, err error) ErrorResult {
		t.Helper()
		res, rErr := errorResult("01JD0000000000000000000000", err)
		if rErr != nil || !res.IsError || res.StructuredContent != nil {
			t.Fatalf("errorResult = %#v, %v", res, rErr)
		}
		var out ErrorResult
		if err := json.Unmarshal([]byte(textContent(t, res)), &out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	for _, code := range prd {
		if got := decode(t, core.NewError(code, "fixed text")); got.Code != code || got.Message != "fixed text" {
			t.Fatalf("code %s: got %+v", code, got)
		}
	}
	for _, err := range []error{
		core.NewError("permission_denied", "agent text"),
		core.NewError(core.CodeNotFound, ""),
		&core.Error{Message: "agent text"},
	} {
		if got := decode(t, err); got.Code != core.CodeInternal || got.Message != messageInternal {
			t.Fatalf("error %v: got %+v; want internal", err, got)
		}
	}
}

func TestNotFoundForForeignResource(t *testing.T) {
	inv := inventory.New()
	for _, e := range []inventory.Entry{
		{Name: "only-b", Kind: "postgres", Capabilities: []string{"sql"}, Agents: []string{"ac29b"}, Resource: &holdUntilCancel{}, Status: inventory.StatusServed},
		{Name: "shared", Kind: "postgres", Capabilities: []string{"sql"}, Agents: []string{"ac29a", "ac29b"}, Resource: &holdUntilCancel{}, Status: inventory.StatusServed},
	} {
		if err := inv.Add(e); err != nil {
			t.Fatal(err)
		}
	}
	withCapabilities(t, sql.Capability())
	p := pipeline.New(pipeline.Options{Inventory: inv})
	handler := auth.RequireBearerToken(subAsToken, nil)(Handler(newServer(inv, p, "test")))
	session := connect(t, handler, "ac29a")

	var bodies []string
	for _, resource := range []string{"only-b", "no-such"} {
		for _, name := range []string{"sql_query", "sql_list_tables", "sql_describe_table"} {
			args := toolArguments(name)
			args["resource"] = resource
			obj := errorObject(t, callTool(t, session, name, args))
			if obj["code"] != string(core.CodeNotFound) || obj["message"] != "resource not found" {
				t.Fatalf("%s on %s = %v; want not_found", name, resource, obj)
			}
			text := textContent(t, callTool(t, session, name, args))
			if strings.Contains(text, resource) {
				t.Fatalf("%s on %s: message names the resource: %s", name, resource, text)
			}
			delete(obj, "request_id")
			raw, _ := json.Marshal(obj)
			bodies = append(bodies, string(raw))
		}
	}
	for _, body := range bodies[1:] {
		if body != bodies[0] {
			t.Fatalf("not_found bodies differ: %v", bodies)
		}
	}

	got := callListResources(t, session)
	if len(got.Resources) != 1 || got.Resources[0].Name != "shared" {
		t.Fatalf("list_resources for ac29a = %+v; want shared only", got.Resources)
	}
	if got := callListResources(t, connect(t, handler, "ac29b")); len(got.Resources) != 2 {
		t.Fatalf("list_resources for ac29b = %+v; want both", got.Resources)
	}
}
