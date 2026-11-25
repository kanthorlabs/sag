package pipeline

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/kanthorlabs/sag/internal/core"
	"github.com/kanthorlabs/sag/internal/inventory"
)

type call struct {
	capability string
	operation  string
	input      json.RawMessage
	lim        core.Limits
}

type fakeResource struct {
	calls  []call
	result core.Result
	err    error
}

func (f *fakeResource) Preflight(context.Context) (core.PreflightReport, error) {
	return core.PreflightReport{Passed: true}, nil
}

func (f *fakeResource) Execute(_ context.Context, capability, operation string, input json.RawMessage, lim core.Limits) (core.Result, error) {
	f.calls = append(f.calls, call{capability: capability, operation: operation, input: input, lim: lim})
	return f.result, f.err
}

func (f *fakeResource) Close() error { return nil }

type laxResolver struct {
	entries map[string]inventory.Entry
}

func (l laxResolver) Lookup(_, name string) (inventory.Entry, error) {
	e, ok := l.entries[name]
	if !ok {
		return inventory.Entry{}, core.NewError(core.CodeNotFound, "resource not found")
	}
	return e, nil
}

type failingResolver struct{}

func (failingResolver) Lookup(string, string) (inventory.Entry, error) {
	return inventory.Entry{}, errors.New("resolver broken")
}

var testLimits = core.Limits{MaxRows: 7, MaxResultBytes: 1024, MaxCellBytes: 64, Timeout: 3 * time.Second}

func entry(name string, status inventory.Status, res core.Resource, agents ...string) inventory.Entry {
	return inventory.Entry{
		Name:         name,
		Kind:         "postgres",
		Capabilities: []string{"sql"},
		Agents:       agents,
		Limits:       testLimits,
		Resource:     res,
		Status:       status,
	}
}

func request(sub, resource string) Request {
	return Request{ID: "req-1", Sub: sub, Resource: resource, Capability: "sql", Operation: "query", Input: json.RawMessage(`{"resource":"` + resource + `","sql":"SELECT 1"}`)}
}

func realInventory(t *testing.T, entries ...inventory.Entry) *inventory.Inventory {
	t.Helper()
	inv := inventory.New()
	for _, e := range entries {
		if err := inv.Add(e); err != nil {
			t.Fatal(err)
		}
	}
	return inv
}

func assertCode(t *testing.T, err error, code core.Code) *core.Error {
	t.Helper()
	var ce *core.Error
	if !errors.As(err, &ce) {
		t.Fatalf("error = %v; want *core.Error with code %s", err, code)
	}
	if ce.Code != code {
		t.Fatalf("code = %s; want %s", ce.Code, code)
	}
	return ce
}

func TestNotFoundForForeignResource(t *testing.T) {
	foreign := &fakeResource{}
	unlisted := &fakeResource{}
	down := &fakeResource{}
	entries := []inventory.Entry{
		entry("foreign", inventory.StatusServed, foreign, "other"),
		entry("unlisted", inventory.StatusServed, unlisted),
		entry("down", inventory.StatusUnavailable, down, "other"),
	}
	lax := laxResolver{entries: map[string]inventory.Entry{}}
	for _, e := range entries {
		lax.entries[e.Name] = e
	}
	resolvers := map[string]Resolver{"inventory": realInventory(t, entries...), "lax": lax}
	for rname, resolver := range resolvers {
		p := New(Options{Inventory: resolver})
		var want *core.Error
		for _, name := range []string{"missing", "foreign", "unlisted", "down"} {
			for _, sub := range []string{"ac29", ""} {
				_, err := p.Execute(context.Background(), request(sub, name))
				got := assertCode(t, err, core.CodeNotFound)
				if want == nil {
					want = got
				}
				if got.Message != want.Message {
					t.Fatalf("%s/%s/%q: message %q differs from %q", rname, name, sub, got.Message, want.Message)
				}
			}
		}
		_, err := New(Options{Inventory: failingResolver{}}).Execute(context.Background(), request("ac29", "foreign"))
		if got := assertCode(t, err, core.CodeNotFound); got.Message != want.Message {
			t.Fatalf("broken resolver: message %q differs from %q", got.Message, want.Message)
		}
	}
	for _, r := range []*fakeResource{foreign, unlisted, down} {
		if len(r.calls) != 0 {
			t.Fatalf("Execute reached a resource without a grant: %#v", r.calls)
		}
	}
}

func TestUnavailableForFailedPreflight(t *testing.T) {
	for _, status := range []inventory.Status{inventory.StatusUnavailable, inventory.StatusPending} {
		res := &fakeResource{}
		e := entry("ext", status, res, "ac11")
		resolvers := map[string]Resolver{
			"inventory": realInventory(t, e),
			"lax":       laxResolver{entries: map[string]inventory.Entry{"ext": e}},
		}
		for rname, resolver := range resolvers {
			_, err := New(Options{Inventory: resolver}).Execute(context.Background(), request("ac11", "ext"))
			if ce := assertCode(t, err, core.CodeResourceUnavailable); ce.Message != "resource unavailable" {
				t.Fatalf("%s/%s: message %q", rname, status, ce.Message)
			}
		}
		if len(res.calls) != 0 {
			t.Fatalf("%s: Execute reached a resource that is not served", status)
		}
	}
	nilResource := entry("bare", inventory.StatusServed, nil, "ac11")
	_, err := New(Options{Inventory: laxResolver{entries: map[string]inventory.Entry{"bare": nilResource}}}).Execute(context.Background(), request("ac11", "bare"))
	assertCode(t, err, core.CodeResourceUnavailable)
}

func TestExecutesWithEntryLimits(t *testing.T) {
	res := &fakeResource{result: core.Result{Columns: []core.Column{{Name: "n", Type: "int4"}}, Rows: [][]any{{int64(1)}}}}
	p := New(Options{Inventory: realInventory(t, entry("main", inventory.StatusServed, res, "ac05"))})
	req := request("ac05", "main")
	got, err := p.Execute(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Rows) != 1 || got.Rows[0][0] != int64(1) {
		t.Fatalf("result = %#v", got)
	}
	if len(res.calls) != 1 {
		t.Fatalf("Execute calls = %d; want 1", len(res.calls))
	}
	c := res.calls[0]
	if c.capability != "sql" || c.operation != "query" || string(c.input) != string(req.Input) || c.lim != testLimits {
		t.Fatalf("Execute call = %#v", c)
	}
}

func TestUnknownCapabilityRejected(t *testing.T) {
	res := &fakeResource{}
	p := New(Options{Inventory: realInventory(t, entry("main", inventory.StatusServed, res, "ac05"))})
	req := request("ac05", "main")
	req.Capability = "logs"
	_, err := p.Execute(context.Background(), req)
	assertCode(t, err, core.CodeInvalidArgument)
	if len(res.calls) != 0 {
		t.Fatal("Execute reached the resource for an unknown capability")
	}
}

func TestExecuteErrorPassesThrough(t *testing.T) {
	want := &core.Error{Code: core.CodeUpstreamError, Message: "upstream error, SQLSTATE 25006"}
	res := &fakeResource{err: want}
	p := New(Options{Inventory: realInventory(t, entry("main", inventory.StatusServed, res, "ac05"))})
	_, err := p.Execute(context.Background(), request("ac05", "main"))
	if !errors.Is(err, want) {
		t.Fatalf("error = %v; want %v", err, want)
	}
}
