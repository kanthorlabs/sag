package inventory

import (
	"errors"
	"testing"

	"github.com/kanthorlabs/sag/internal/core"
)

func served(name string, agents ...string) Entry {
	return Entry{Name: name, Kind: "postgres", Agents: agents, Status: StatusServed}
}

func mustAdd(t *testing.T, inv *Inventory, e Entry) {
	t.Helper()
	if err := inv.Add(e); err != nil {
		t.Fatalf("Add(%s): %v", e.Name, err)
	}
}

func names(entries []Entry) []string {
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.Name)
	}
	return out
}

func code(err error) core.Code {
	var ce *core.Error
	if errors.As(err, &ce) {
		return ce.Code
	}
	return ""
}

func TestVisibleDefaultDeny(t *testing.T) {
	inv := New()
	mustAdd(t, inv, served("no-agents"))
	mustAdd(t, inv, served("other", "bob"))
	mustAdd(t, inv, served("zeta", "alice"))
	mustAdd(t, inv, served("alpha", "carol", "alice"))
	pending := served("pending", "alice")
	pending.Status = StatusPending
	mustAdd(t, inv, pending)
	mustAdd(t, inv, served("down", "alice"))
	if err := inv.SetStatus("down", StatusUnavailable); err != nil {
		t.Fatal(err)
	}

	got := names(inv.Visible("alice"))
	if len(got) != 2 || got[0] != "alpha" || got[1] != "zeta" {
		t.Fatalf("Visible(alice) = %v; want [alpha zeta]", got)
	}
	if got := inv.Visible("mallory"); len(got) != 0 {
		t.Fatalf("Visible(mallory) = %v; want none", names(got))
	}
	if got := inv.Visible(""); len(got) != 0 {
		t.Fatalf("Visible(\"\") = %v; want none", names(got))
	}
}

func TestVisibleAfterStatusChange(t *testing.T) {
	inv := New()
	e := served("main", "alice")
	e.Status = StatusPending
	mustAdd(t, inv, e)
	if got := inv.Visible("alice"); len(got) != 0 {
		t.Fatalf("pending resource is visible: %v", names(got))
	}
	if err := inv.SetStatus("main", StatusServed); err != nil {
		t.Fatal(err)
	}
	if got := names(inv.Visible("alice")); len(got) != 1 || got[0] != "main" {
		t.Fatalf("Visible(alice) = %v; want [main]", got)
	}
}

func TestAddCopiesAgents(t *testing.T) {
	inv := New()
	agents := []string{"alice"}
	mustAdd(t, inv, Entry{Name: "main", Agents: agents, Status: StatusServed})
	agents[0] = "mallory"
	if got := inv.Visible("mallory"); len(got) != 0 {
		t.Fatalf("caller mutation changed authorization: %v", names(got))
	}
	got := inv.Visible("alice")
	if len(got) != 1 {
		t.Fatalf("Visible(alice) = %v", names(got))
	}
	got[0].Agents[0] = "mallory"
	if got := inv.Visible("mallory"); len(got) != 0 {
		t.Fatalf("result mutation changed authorization: %v", names(got))
	}
}

func TestAddRejectsDuplicateAndEmptyName(t *testing.T) {
	inv := New()
	mustAdd(t, inv, served("main", "alice"))
	if err := inv.Add(served("main", "bob")); err == nil {
		t.Fatal("Add accepted a duplicate name")
	}
	if err := inv.Add(served("", "bob")); err == nil {
		t.Fatal("Add accepted an empty name")
	}
}

func TestSetStatusUnknown(t *testing.T) {
	inv := New()
	if err := inv.SetStatus("ghost", StatusServed); err == nil {
		t.Fatal("SetStatus accepted an unknown name")
	}
}

func TestLookupNotFound(t *testing.T) {
	inv := New()
	mustAdd(t, inv, served("main", "alice"))
	mustAdd(t, inv, served("no-agents"))

	_, unknown := inv.Lookup("alice", "ghost")
	_, foreign := inv.Lookup("bob", "main")
	_, empty := inv.Lookup("alice", "no-agents")
	_, noSub := inv.Lookup("", "main")
	for _, err := range []error{unknown, foreign, empty, noSub} {
		if code(err) != core.CodeNotFound {
			t.Fatalf("Lookup error = %v; want not_found", err)
		}
	}
	if unknown.Error() != foreign.Error() {
		t.Fatalf("unknown %q and foreign %q differ", unknown, foreign)
	}
}

func TestLookupStatus(t *testing.T) {
	inv := New()
	mustAdd(t, inv, served("main", "alice"))
	pending := served("pending", "alice")
	pending.Status = StatusPending
	mustAdd(t, inv, pending)
	mustAdd(t, inv, served("down", "alice"))
	if err := inv.SetStatus("down", StatusUnavailable); err != nil {
		t.Fatal(err)
	}

	e, err := inv.Lookup("alice", "main")
	if err != nil || e.Name != "main" {
		t.Fatalf("Lookup(alice, main) = %v, %v", e.Name, err)
	}
	for _, name := range []string{"pending", "down"} {
		_, err := inv.Lookup("alice", name)
		if code(err) != core.CodeResourceUnavailable {
			t.Fatalf("Lookup(alice, %s) error = %v; want resource_unavailable", name, err)
		}
	}
	_, err = inv.Lookup("bob", "down")
	if code(err) != core.CodeNotFound {
		t.Fatalf("Lookup(bob, down) error = %v; want not_found", err)
	}
}
