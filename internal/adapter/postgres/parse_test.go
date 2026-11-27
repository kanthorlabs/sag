package postgres

import (
	"testing"

	"github.com/kanthorlabs/sag/internal/approval"
	"github.com/kanthorlabs/sag/internal/core"
)

func TestParseRejectsInvalidText(t *testing.T) {
	_, err := parse("SELEC 1")
	assertQueryRejected(t, err)
	_, err = newDialect(core.ResourceConfig{}, approval.Target{}).Parse("SELECT FROM WHERE (")
	assertQueryRejected(t, err)
}

func TestRenderDeparses(t *testing.T) {
	tree, err := parse("select  a ,b from   t /* note */ where c = 'it''s' -- tail")
	if err != nil {
		t.Fatal(err)
	}
	got, err := render(tree)
	if err != nil {
		t.Fatal(err)
	}
	want := "SELECT a, b FROM t WHERE c = 'it''s'"
	if got != want {
		t.Fatalf("Render() = %q; want %q", got, want)
	}
}

func TestRenderRejectsForeignTree(t *testing.T) {
	d := newDialect(core.ResourceConfig{}, approval.Target{})
	_, err := d.Render("SELECT 1")
	assertQueryRejected(t, err)
	_, err = render(nil)
	assertQueryRejected(t, err)
}
