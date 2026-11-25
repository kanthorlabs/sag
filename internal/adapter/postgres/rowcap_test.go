package postgres

import (
	"testing"

	"github.com/kanthorlabs/sag/internal/approval"
	"github.com/kanthorlabs/sag/internal/core"
	"github.com/kanthorlabs/sag/internal/sql"
)

func renderedTree(t *testing.T, text string) (sql.Tree, string) {
	t.Helper()
	tree, err := parse(text)
	if err != nil {
		t.Fatalf("parse(%q): %v", text, err)
	}
	rendered, err := render(tree)
	if err != nil {
		t.Fatalf("render(%q): %v", text, err)
	}
	return &pgTree{Result: tree, Validated: true}, rendered
}

func TestSelectPlanUsesCursor(t *testing.T) {
	d := newDialect(core.ResourceConfig{}, approval.Target{})
	for _, text := range []string{
		"SELECT a FROM t WHERE b = 'x'",
		"WITH a AS (SELECT 1 AS x) SELECT x FROM a",
		"SELECT 1 UNION ALL SELECT 2",
		"VALUES (1), (2)",
	} {
		tree, rendered := renderedTree(t, text)
		plan, err := d.PlanRowCap(tree, rendered, 100)
		if err != nil {
			t.Fatalf("PlanRowCap(%q): %v", text, err)
		}
		want := sql.RowCapPlan{Statement: "DECLARE sag_c NO SCROLL CURSOR FOR " + rendered, Fetch: "FETCH 101 FROM sag_c"}
		if plan != want {
			t.Fatalf("PlanRowCap(%q) = %+v; want %+v", text, plan, want)
		}
	}
}

func TestExplainPlanNoFetch(t *testing.T) {
	d := newDialect(core.ResourceConfig{}, approval.Target{})
	tree, rendered := renderedTree(t, "EXPLAIN (FORMAT JSON) SELECT a FROM t")
	plan, err := d.PlanRowCap(tree, rendered, 5)
	if err != nil {
		t.Fatal(err)
	}
	want := sql.RowCapPlan{Statement: rendered, Fetch: ""}
	if plan != want {
		t.Fatalf("PlanRowCap() = %+v; want %+v", plan, want)
	}
}

func TestPlanRowCapRejects(t *testing.T) {
	d := newDialect(core.ResourceConfig{}, approval.Target{})
	selectTree, selectText := renderedTree(t, "SELECT 1")
	multiTree, multiText := renderedTree(t, "SELECT 1; SELECT 2")
	insertTree, insertText := renderedTree(t, "INSERT INTO t VALUES (1)")
	cases := []struct {
		name     string
		tree     sql.Tree
		rendered string
		maxRows  int
	}{
		{"multi statement tree", multiTree, multiText, 10},
		{"insert", insertTree, insertText, 10},
		{"rendered text with a second statement", selectTree, "SELECT 1; DELETE FROM t", 10},
		{"rendered text that does not parse", selectTree, "SELECT (", 10},
		{"zero max rows", selectTree, selectText, 0},
		{"foreign tree", "SELECT 1", selectText, 10},
		{"nil tree", nil, selectText, 10},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := d.PlanRowCap(c.tree, c.rendered, c.maxRows)
			assertQueryRejected(t, err)
		})
	}
}
