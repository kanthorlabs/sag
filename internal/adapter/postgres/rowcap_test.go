package postgres

import (
	"fmt"
	"math"
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

func TestFetchIsMaxRowsPlusOne(t *testing.T) {
	d := newDialect(core.ResourceConfig{}, approval.Target{})
	for _, text := range []string{
		"SELECT a FROM t WHERE b = 'x'",
		"WITH a AS (SELECT 1 AS x) SELECT x FROM a",
		"SELECT 1 UNION ALL SELECT 2",
		"VALUES (1), (2)",
		"SELECT E'a\\\\', ' , pg_sleep(2) -- '",
	} {
		for _, maxRows := range []int{1, 100, math.MaxInt32 - 1} {
			tree, rendered := renderedTree(t, text)
			plan, err := d.PlanRowCap(tree, rendered, maxRows)
			if err != nil {
				t.Fatalf("PlanRowCap(%q, %d): %v", text, maxRows, err)
			}
			want := sql.RowCapPlan{Statement: "DECLARE sag_c NO SCROLL CURSOR FOR " + rendered, Fetch: fmt.Sprintf("FETCH %d FROM sag_c", maxRows+1)}
			if plan != want {
				t.Fatalf("PlanRowCap(%q, %d) = %+v; want %+v", text, maxRows, plan, want)
			}
		}
	}
}

func TestExplainNoCursor(t *testing.T) {
	d := newDialect(core.ResourceConfig{}, approval.Target{})
	for _, text := range []string{
		"EXPLAIN (FORMAT JSON) SELECT a FROM t",
		"EXPLAIN SELECT 1 UNION SELECT 2",
		"EXPLAIN VERBOSE WITH a AS (SELECT 1) SELECT * FROM a",
	} {
		tree, rendered := renderedTree(t, text)
		plan, err := d.PlanRowCap(tree, rendered, 5)
		if err != nil {
			t.Fatalf("PlanRowCap(%q): %v", text, err)
		}
		want := sql.RowCapPlan{Statement: rendered, Fetch: ""}
		if plan != want {
			t.Fatalf("PlanRowCap(%q) = %+v; want %+v", text, plan, want)
		}
	}
}

func TestOneDeclareRequired(t *testing.T) {
	d := newDialect(core.ResourceConfig{}, approval.Target{})
	selectTree, selectText := renderedTree(t, "SELECT 1")
	explainTree, explainText := renderedTree(t, "EXPLAIN SELECT 1")
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
		{"rendered text that closes the cursor query", selectTree, "SELECT 1; DECLARE sag_c NO SCROLL CURSOR FOR SELECT 1", 10},
		{"rendered text that does not parse", selectTree, "SELECT (", 10},
		{"rendered text of another statement", selectTree, "SELECT 2", 10},
		{"rendered text with a row lock", selectTree, "SELECT 1 FOR UPDATE", 10},
		{"rendered text with a hold cursor option", selectTree, "SELECT 1 WITH HOLD", 10},
		{"empty rendered text", selectTree, "", 10},
		{"explain with another rendered statement", explainTree, "EXPLAIN SELECT 2", 10},
		{"explain with a second statement", explainTree, explainText + "; SELECT 2", 10},
		{"explain with analyze", explainTree, "EXPLAIN ANALYZE SELECT 1", 10},
		{"select rendered as explain", selectTree, explainText, 10},
		{"zero max rows", selectTree, selectText, 0},
		{"negative max rows", selectTree, selectText, -1},
		{"max rows at the FETCH limit", selectTree, selectText, math.MaxInt32},
		{"max rows above the FETCH limit", selectTree, selectText, math.MaxInt},
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

func TestOneDeclareRequiresValidatedTree(t *testing.T) {
	for _, tree := range []*pgTree{nil, {}, mustParseTree(t, "SELECT 1")} {
		_, err := planRowCap(tree, "SELECT 1", 10)
		assertQueryRejected(t, err)
	}
}
