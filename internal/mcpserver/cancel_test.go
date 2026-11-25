package mcpserver

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	_ "github.com/kanthorlabs/sag/internal/adapter/postgres"
	"github.com/kanthorlabs/sag/internal/core"
	"github.com/kanthorlabs/sag/internal/inventory"
	"github.com/kanthorlabs/sag/internal/pipeline"
	"github.com/kanthorlabs/sag/internal/sql"
)

type holdUntilCancel struct {
	calls    atomic.Int32
	entered  chan struct{}
	released chan struct{}
	stop     chan struct{}
}

func (h *holdUntilCancel) Preflight(context.Context) (core.PreflightReport, error) {
	return core.PreflightReport{Passed: true}, nil
}

func (h *holdUntilCancel) Execute(ctx context.Context, _, _ string, _ json.RawMessage, _ core.Limits) (core.Result, error) {
	if h.calls.Add(1) > 1 {
		return core.Result{}, nil
	}
	h.entered <- struct{}{}
	select {
	case <-ctx.Done():
		h.released <- struct{}{}
		return core.Result{}, ctx.Err()
	case <-h.stop:
		return core.Result{}, nil
	}
}

func (h *holdUntilCancel) Close() error { return nil }

func TestClientDisconnectReleasesSlot(t *testing.T) {
	res := &holdUntilCancel{entered: make(chan struct{}, 1), released: make(chan struct{}, 1), stop: make(chan struct{})}
	inv := inventory.New()
	if err := inv.Add(inventory.Entry{Name: "main", Kind: "postgres", Capabilities: []string{"sql"}, Agents: []string{"c4"}, Resource: res, Status: inventory.StatusServed}); err != nil {
		t.Fatal(err)
	}
	withCapabilities(t, sql.Capability())
	p := pipeline.New(pipeline.Options{Inventory: inv, MaxConcurrentPerAgent: 1})
	session := connect(t, auth.RequireBearerToken(subAsToken, nil)(Handler(newServer(inv, p, "test"))), "c4")
	t.Cleanup(func() { close(res.stop) })
	args := map[string]any{"resource": "main", "sql": "SELECT 1"}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		session.CallTool(ctx, &mcp.CallToolParams{Name: "sql_query", Arguments: args})
	}()
	select {
	case <-res.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("first call did not reach the resource")
	}
	cancel()
	select {
	case <-res.released:
	case <-time.After(5 * time.Second):
		t.Fatal("client disconnect did not cancel the handler context")
	}
	<-done

	deadline := time.Now().Add(5 * time.Second)
	for {
		got := callTool(t, session, "sql_query", args)
		if !got.IsError {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("slot not released: %v", errorObject(t, got))
		}
		time.Sleep(10 * time.Millisecond)
	}
}
