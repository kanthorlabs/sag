package postgres

import (
	"context"
	"errors"
	"testing"

	"github.com/kanthorlabs/sag/internal/approval"
	"github.com/kanthorlabs/sag/internal/core"
)

func assertQueryRejected(t *testing.T, err error) {
	t.Helper()
	var coreErr *core.Error
	if !errors.As(err, &coreErr) || coreErr.Code != core.CodeQueryRejected {
		t.Fatalf("error = %v; want core.Error %s", err, core.CodeQueryRejected)
	}
}

func TestFailClosedRejectsAll(t *testing.T) {
	d := newDialect(core.ResourceConfig{}, approval.Target{})
	for _, text := range []string{
		"SELECT 1",
		"SELECT a FROM t WHERE b = 'x'",
		"WITH a AS (SELECT 1) SELECT * FROM a",
		"EXPLAIN SELECT 1",
		"INSERT INTO t VALUES (1)",
		"SELECT 1; SELECT 2",
	} {
		tree, err := parse(text)
		if err != nil {
			t.Fatalf("parse(%q): %v", text, err)
		}
		assertQueryRejected(t, validateStatic(tree))
		assertQueryRejected(t, d.ValidateStatic(tree))
	}
	assertQueryRejected(t, validateStatic(nil))
	assertQueryRejected(t, d.ValidateStatic(nil))
	assertQueryRejected(t, d.ValidateStatic("SELECT 1"))
}

func TestFailClosedValidateCatalog(t *testing.T) {
	d := newDialect(core.ResourceConfig{}, approval.Target{})
	tree, err := parse("SELECT 1")
	if err != nil {
		t.Fatal(err)
	}
	assertQueryRejected(t, d.ValidateCatalog(context.Background(), nil, tree))
}
