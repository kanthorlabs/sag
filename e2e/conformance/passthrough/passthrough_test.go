package passthrough

import (
	"context"
	"errors"
	"testing"

	"github.com/kanthorlabs/sag/internal/core"
	"github.com/kanthorlabs/sag/internal/sql"
)

type innerDialect struct {
	sql.Dialect
}

func (innerDialect) EngineGuard() sql.EngineGuard {
	return sql.EngineGuardTransaction
}

func (innerDialect) SessionGuard() []string {
	return []string{"SET default_transaction_read_only = on"}
}

func (innerDialect) RequestBegin(core.Limits) []string {
	return []string{"BEGIN TRANSACTION READ ONLY"}
}

func (innerDialect) ValidateStatic(sql.Tree) error {
	return errors.New("inner validator")
}

const passText = "SELECT 1; DELETE FROM t"

func TestPassesTextUnchanged(t *testing.T) {
	d := Wrap(innerDialect{})

	tree, err := d.Parse(passText)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if err := d.ValidateStatic(tree); err != nil {
		t.Fatalf("ValidateStatic: %v", err)
	}
	rendered, err := d.Render(tree)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if rendered != passText {
		t.Fatalf("Render = %q, want %q", rendered, passText)
	}
	if err := d.ValidateCatalog(context.Background(), nil, tree); err != nil {
		t.Fatalf("ValidateCatalog: %v", err)
	}
	plan, err := d.PlanRowCap(tree, rendered, 10)
	if err != nil {
		t.Fatalf("PlanRowCap: %v", err)
	}
	if plan != (sql.RowCapPlan{Statement: passText}) {
		t.Fatalf("PlanRowCap = %+v, want Statement %q and empty Fetch", plan, passText)
	}
}

func TestRenderRejectsForeignTree(t *testing.T) {
	if _, err := Wrap(innerDialect{}).Render(42); err == nil {
		t.Fatal("Render of a non-text tree: no error")
	}
}

func TestKeepsC2FromInnerDialect(t *testing.T) {
	d := Wrap(innerDialect{})
	if d.EngineGuard() != sql.EngineGuardTransaction {
		t.Fatalf("EngineGuard = %q", d.EngineGuard())
	}
	if got := d.SessionGuard(); len(got) != 1 || got[0] != "SET default_transaction_read_only = on" {
		t.Fatalf("SessionGuard = %q", got)
	}
	if got := d.RequestBegin(core.Limits{}); len(got) != 1 || got[0] != "BEGIN TRANSACTION READ ONLY" {
		t.Fatalf("RequestBegin = %q", got)
	}
}
