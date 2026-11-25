package postgres

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kanthorlabs/sag/internal/approval"
	"github.com/kanthorlabs/sag/internal/core"
	"github.com/kanthorlabs/sag/internal/sql"
)

func rejectionMessage(reason string) string {
	rule, _, _ := strings.Cut(reason, ":")
	return rejectionPrefix + rule
}

func assertRejectionReason(t *testing.T, err error, want string) {
	t.Helper()
	var rejection *Rejection
	if !errors.As(err, &rejection) || rejection.Reason() != want {
		t.Fatalf("error = %v; want rejection reason %q", err, want)
	}
}

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
	if tree.Validated || tree.Funcs != nil || tree.FieldNames != nil || tree.Operators != nil {
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
	tree.FieldNames = []qualifiedName{{Name: "c"}}
	assertQueryRejected(t, emptyDialect().ValidateStatic(tree))
	if tree.Validated {
		t.Fatal("ValidateStatic() kept Validated after a rejection")
	}
	if tree.Funcs != nil || tree.FieldNames != nil || tree.Operators != nil {
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

func TestReadCorpusFullStaticPath(t *testing.T) {
	d := emptyDialect()
	for _, text := range readCorpus {
		tree := mustParseTree(t, text)
		if err := d.ValidateStatic(tree); err != nil {
			t.Errorf("ValidateStatic(%q): %v", text, err)
			continue
		}
		rendered, err := d.Render(tree)
		if err != nil {
			t.Errorf("Render(%q): %v", text, err)
			continue
		}
		again := mustParseTree(t, rendered)
		if err := d.ValidateStatic(again); err != nil {
			t.Errorf("ValidateStatic(Render(%q) = %q): %v", text, rendered, err)
			continue
		}
		if _, err := d.PlanRowCap(again, rendered, 10); err != nil {
			t.Errorf("PlanRowCap(%q): %v", rendered, err)
		}
	}
}

func assertStaticReason(t *testing.T, text, want string) {
	t.Helper()
	tree := mustParseTree(t, text)
	err := emptyDialect().ValidateStatic(tree)
	var coreErr *core.Error
	if !errors.As(err, &coreErr) || coreErr.Code != core.CodeQueryRejected || coreErr.Message != rejectionMessage(want) {
		t.Fatalf("ValidateStatic(%q) = %v; want query_rejected %q", text, err, rejectionMessage(want))
	}
	assertRejectionReason(t, err, want)
	if tree.Validated {
		t.Fatalf("ValidateStatic(%q) marked the tree as validated", text)
	}
}

func TestValidateStaticEachCheckOnPath(t *testing.T) {
	for _, c := range []struct{ check, text, want string }{
		{"checkStatementShape", "SELECT 1; SELECT 2", "statement_count:2"},
		{"checkStatementShape", "EXPLAIN (ANALYZE) SELECT 1", "explain_option:analyze"},
		{"walkAllowlist", "SELECT $1", "node:ParamRef"},
		{"walkAllowlist", "SELECT B'1'", "A_Const.bsval"},
		{"walkAllowlist", "SELECT * FROM t TABLESAMPLE SYSTEM (50)", "node:RangeTableSample"},
		{"checkRelations", "SELECT * FROM pg_authid", "relation:pg_authid"},
		{"checkFunctionNames", "SELECT pg_sleep(1)", "function_prefix:pg_sleep"},
		{"checkFunctionNames", "SELECT ('x'::text).current_setting", "function_denied:current_setting"},
	} {
		t.Run(c.check+" "+c.text, func(t *testing.T) {
			assertStaticReason(t, c.text, c.want)
		})
	}
}

func TestDeepTreeRejected(t *testing.T) {
	shapes := map[string]func(int) string{
		"array": func(n int) string { return "SELECT ARRAY" + strings.Repeat("[", n) + "1" + strings.Repeat("]", n) },
		"call":  func(n int) string { return "SELECT " + strings.Repeat("a(", n) + "1" + strings.Repeat(")", n) },
		"cast":  func(n int) string { return "SELECT 1" + strings.Repeat("::a", n) },
		"plus":  func(n int) string { return "SELECT 1" + strings.Repeat("+1", n) },
		"sub":   func(n int) string { return "SELECT " + strings.Repeat("(SELECT ", n) + "1" + strings.Repeat(")", n) },
	}
	for name, shape := range shapes {
		t.Run(name, func(t *testing.T) {
			for _, levels := range []int{4995, 4700, 1000} {
				text := shape(levels)
				if len(text) > sql.MaxSQLBytes {
					continue
				}
				if _, err := emptyDialect().Parse(text); err != nil {
					continue
				}
				assertStaticReason(t, text, "depth:500")
			}
			n := 1
			for len(shape(n+1)) <= sql.MaxSQLBytes {
				n++
			}
			text := shape(n)
			tree, err := emptyDialect().Parse(text)
			if err != nil {
				var coreErr *core.Error
				if !errors.As(err, &coreErr) || coreErr.Message != msgParseFailed {
					t.Fatalf("Parse(%d bytes) = %v; want %q", len(text), err, msgParseFailed)
				}
				return
			}
			assertQueryRejected(t, emptyDialect().ValidateStatic(tree))
		})
	}
	t.Run("limit", func(t *testing.T) {
		d := emptyDialect()
		below := mustParseTree(t, shapes["plus"](245))
		if err := d.ValidateStatic(below); err != nil {
			t.Fatalf("ValidateStatic(245 additions) = %v; want nil", err)
		}
		if _, err := d.Render(below); err != nil {
			t.Fatalf("Render(245 additions) = %v; want nil", err)
		}
		assertStaticReason(t, shapes["plus"](246), "depth:500")
	})
}
