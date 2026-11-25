package postgres

import (
	"context"
	"errors"
	"testing"

	"github.com/kanthorlabs/sag/internal/approval"
	"github.com/kanthorlabs/sag/internal/core"
	"github.com/kanthorlabs/sag/internal/sql"
)

func assertQueryRejected(t *testing.T, err error) {
	t.Helper()
	var coreErr *core.Error
	if !errors.As(err, &coreErr) || coreErr.Code != core.CodeQueryRejected {
		t.Fatalf("error = %v; want core.Error %s", err, core.CodeQueryRejected)
	}
}

func emptyDialect() *dialect {
	return newDialect(core.ResourceConfig{}, approval.Target{})
}

func mustParseTree(t *testing.T, text string) *pgTree {
	t.Helper()
	tree, err := emptyDialect().Parse(text)
	if err != nil {
		t.Fatalf("Parse(%q): %v", text, err)
	}
	pt, ok := tree.(*pgTree)
	if !ok {
		t.Fatalf("Parse(%q) = %T; want *pgTree", text, tree)
	}
	return pt
}

func TestParseReturnsPGTree(t *testing.T) {
	tree := mustParseTree(t, "SELECT 1")
	if tree.Result == nil || len(tree.Result.GetStmts()) != 1 {
		t.Fatalf("Parse() result = %+v; want one statement", tree.Result)
	}
	if tree.Validated || tree.Funcs != nil || tree.Operators != nil || tree.Relations != nil {
		t.Fatalf("Parse() = %+v; want an unvalidated tree without names", tree)
	}
}

func TestParseRejectsNUL(t *testing.T) {
	for _, text := range []string{"SELECT 1\x00; DROP TABLE t", "\x00", "SELECT '\x00'"} {
		_, err := emptyDialect().Parse(text)
		assertQueryRejected(t, err)
		var coreErr *core.Error
		if errors.As(err, &coreErr) && coreErr.Message != msgNULByte {
			t.Fatalf("Parse(%q) message = %q; want %q", text, coreErr.Message, msgNULByte)
		}
	}
}

func TestValidateStaticFailClosed(t *testing.T) {
	d := emptyDialect()
	for _, text := range []string{
		"INSERT INTO t VALUES (1)",
		"SELECT 1; SELECT 2",
		"DELETE FROM t",
		"SELECT * FROM t FOR UPDATE",
	} {
		tree := mustParseTree(t, text)
		assertQueryRejected(t, d.ValidateStatic(tree))
		if tree.Validated {
			t.Fatalf("ValidateStatic(%q) marked the tree as validated", text)
		}
	}
	for _, tree := range []sql.Tree{nil, "SELECT 1", (*pgTree)(nil), &pgTree{}} {
		assertQueryRejected(t, d.ValidateStatic(tree))
	}
}

func TestValidateStaticClearsStaleState(t *testing.T) {
	tree := mustParseTree(t, "INSERT INTO t VALUES (1)")
	tree.Validated = true
	tree.Funcs = []qualifiedName{{Name: "f"}}
	tree.Operators = []qualifiedName{{Name: "="}}
	tree.Relations = []qualifiedName{{Name: "t"}}
	assertQueryRejected(t, emptyDialect().ValidateStatic(tree))
	if tree.Validated {
		t.Fatal("ValidateStatic() kept Validated after a rejection")
	}
	if tree.Funcs != nil || tree.Operators != nil || tree.Relations != nil {
		t.Fatalf("ValidateStatic() kept names from a previous run: %+v", tree)
	}
}

func TestLaterStagesNeedValidatedTree(t *testing.T) {
	d := emptyDialect()
	for _, tree := range []sql.Tree{nil, "SELECT 1", (*pgTree)(nil), &pgTree{}, mustParseTree(t, "SELECT 1")} {
		_, err := d.Render(tree)
		assertQueryRejected(t, err)
		assertQueryRejected(t, d.ValidateCatalog(context.Background(), nil, tree))
		_, err = d.PlanRowCap(tree, "SELECT 1", 10)
		assertQueryRejected(t, err)
	}
}

func TestQueryRejectedMapping(t *testing.T) {
	if queryRejected(nil) != nil {
		t.Fatal("queryRejected(nil) != nil")
	}
	rejection := &Rejection{Node: "SelectStmt", Field: "locking_clause"}
	var coreErr *core.Error
	if err := queryRejected(rejection); !errors.As(err, &coreErr) || coreErr.Code != core.CodeQueryRejected || coreErr.Message != "statement rejected: SelectStmt.locking_clause" || !errors.Is(err, rejection) {
		t.Fatalf("queryRejected(rejection) = %#v", err)
	}
	engineErr := &sql.EngineError{SQLState: "57014", Class: sql.Timeout}
	if err := queryRejected(engineErr); err != engineErr {
		t.Fatalf("queryRejected(engine error) = %v; want it unchanged", err)
	}
	if err := queryRejected(context.DeadlineExceeded); err != context.DeadlineExceeded {
		t.Fatalf("queryRejected(deadline) = %v; want it unchanged", err)
	}
	given := core.NewError(core.CodeTimeout, "request timed out")
	if err := queryRejected(given); err != given {
		t.Fatalf("queryRejected(core error) = %v; want it unchanged", err)
	}
	other := errors.New("rows closed")
	if err := queryRejected(other); !errors.As(err, &coreErr) || coreErr.Code != core.CodeQueryRejected || coreErr.Message != msgRejected || !errors.Is(err, other) {
		t.Fatalf("queryRejected(other) = %#v", err)
	}
}
