//go:build integration

package postgres

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kanthorlabs/sag/internal/sql"
	"github.com/kanthorlabs/sag/internal/testenv"
)

const firstMajorWithFixedSCS = 19

func adminExec(t *testing.T, pg *testenv.Postgres, statement string) error {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	conn, err := pg.Connect(ctx)
	if err != nil {
		t.Fatalf("admin connect: %v", err)
	}
	defer closeConn(ctx, conn)
	return execText(ctx, conn, statement)
}

func hostileSessionSettings(t *testing.T, pg *testenv.Postgres) {
	t.Helper()
	for _, statement := range []string{
		"ALTER ROLE postgres SET standard_conforming_strings = off",
		"ALTER DATABASE postgres SET standard_conforming_strings = off",
	} {
		err := adminExec(t, pg, statement)
		if pg.Major < firstMajorWithFixedSCS && err != nil {
			t.Fatalf("%s on PostgreSQL %d: %v", statement, pg.Major, err)
		}
		if pg.Major >= firstMajorWithFixedSCS && err == nil {
			t.Fatalf("%s on PostgreSQL %d succeeded; want an error", statement, pg.Major)
		}
	}
	if err := adminExec(t, pg, "ALTER ROLE postgres SET client_encoding = 'SJIS'"); err != nil {
		t.Fatalf("ALTER ROLE client_encoding: %v", err)
	}
}

func without(statements []string, settings ...string) []string {
	return slices.DeleteFunc(slices.Clone(statements), func(s string) bool {
		return slices.ContainsFunc(settings, func(setting string) bool { return strings.Contains(s, setting) })
	})
}

func rowStrings(t *testing.T, values []any) []string {
	t.Helper()
	out := make([]string, len(values))
	for i, v := range values {
		s, ok := v.(string)
		if !ok {
			t.Fatalf("column %d = %#v; want a string", i, v)
		}
		out[i] = s
	}
	return out
}

func TestRenderedTextLexerSettings(t *testing.T) {
	pg := startPostgres(t)
	hostileSessionSettings(t, pg)
	cfg := testResourceConfig(pg, 1)
	p := openPool(t, pg, cfg)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	c := acquire(t, ctx, p)
	defer c.Close()

	wantSCS := "off"
	if pg.Major >= firstMajorWithFixedSCS {
		wantSCS = "on"
	}
	if got := show(t, ctx, c, "standard_conforming_strings"); got != wantSCS {
		t.Fatalf("session standard_conforming_strings = %q; want %q", got, wantSCS)
	}
	if got := show(t, ctx, c, "client_encoding"); got != "UTF8" {
		t.Fatalf("session client_encoding = %q; want UTF8 from the startup parameter over the role setting", got)
	}
	if err := c.Exec(ctx, "SET client_encoding = 'SJIS'"); err != nil {
		t.Fatalf("SET client_encoding: %v", err)
	}
	if got := show(t, ctx, c, "client_encoding"); got != "SJIS" {
		t.Fatalf("session client_encoding = %q; want SJIS", got)
	}

	if pg.Major < firstMajorWithFixedSCS {
		const control = `SELECT 'a\', ' , 913 -- '`
		for _, s := range without(requestBegin(testLimits, cfg), "standard_conforming_strings") {
			if err := c.Exec(ctx, s); err != nil {
				t.Fatalf("begin %q: %v", s, err)
			}
		}
		values, err := queryValues(ctx, c, control)
		rollback(t, ctx, c)
		if err != nil {
			t.Fatalf("control %q: %v", control, err)
		}
		if len(values) != 2 || values[0] != `a', ` {
			t.Fatalf("control %q = %#v; want the engine to read %q as the first string", control, values, `a', `)
		}
	}

	begins := map[string][]string{
		"request begin":            requestBegin(testLimits, cfg),
		"session scs":              without(requestBegin(testLimits, cfg), "standard_conforming_strings"),
		"session scs and encoding": without(requestBegin(testLimits, cfg), "standard_conforming_strings", "client_encoding"),
	}
	for _, hc := range hostileLexerCases() {
		tree := markedTree(t, hc.text)
		rendered, again, err := renderAndReparse(tree.Result)
		if err != nil {
			t.Fatalf("renderAndReparse(%q): %v", hc.text, err)
		}
		want := stringConstants(t, again.Result)
		if len(want) == 0 || len(again.Result.Stmts[0].Stmt.GetSelectStmt().GetFromClause()) > 0 {
			continue
		}
		for name, begin := range begins {
			if name == "session scs and encoding" && !isASCII(rendered) {
				continue
			}
			for _, s := range begin {
				if err := c.Exec(ctx, s); err != nil {
					t.Fatalf("%s: begin %q: %v", name, s, err)
				}
			}
			start := time.Now()
			values, err := queryValues(ctx, c, rendered)
			elapsed := time.Since(start)
			rollback(t, ctx, c)
			if err != nil {
				t.Fatalf("%s: %q: %v", name, rendered, err)
			}
			if got := rowStrings(t, values); !slices.Equal(got, want) {
				t.Fatalf("%s: engine read %q as %q; the validator read %q", name, rendered, got, want)
			}
			if elapsed > time.Second {
				t.Fatalf("%s: %q took %s; want less than 1 s", name, rendered, elapsed)
			}
		}
	}
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
}

func TestRowCapPlanOnServer(t *testing.T) {
	pg := startPostgres(t)
	cfg := testResourceConfig(pg, 1)
	p := openPool(t, pg, cfg)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	c := acquire(t, ctx, p)
	defer c.Close()

	tree, rendered := renderedTree(t, "SELECT g FROM pg_catalog.generate_series(1, 50) AS g")
	plan, err := planRowCap(tree.(*pgTree), rendered, 5)
	if err != nil {
		t.Fatalf("planRowCap: %v", err)
	}
	beginRequest(t, ctx, c, testLimits, cfg)
	if err := c.Exec(ctx, plan.Statement); err != nil {
		t.Fatalf("%q: %v", plan.Statement, err)
	}
	rows, err := c.Query(ctx, plan.Fetch)
	if err != nil {
		t.Fatalf("%q: %v", plan.Fetch, err)
	}
	n := 0
	for rows.Next() {
		n++
	}
	rows.Close()
	if err := rows.Err(); err != nil || n != 6 {
		t.Fatalf("%q read %d rows, %v; want 6 rows", plan.Fetch, n, err)
	}
	_, err = queryValues(ctx, c, "FETCH BACKWARD 1 FROM sag_c")
	assertEngineError(t, err, "55000", sql.Other)
	rollback(t, ctx, c)

	tree, rendered = renderedTree(t, "EXPLAIN (FORMAT JSON) SELECT 1")
	plan, err = planRowCap(tree.(*pgTree), rendered, 5)
	if err != nil || plan.Fetch != "" {
		t.Fatalf("planRowCap(EXPLAIN) = %+v, %v; want no cursor", plan, err)
	}
	beginRequest(t, ctx, c, testLimits, cfg)
	values, err := queryValues(ctx, c, plan.Statement)
	rollback(t, ctx, c)
	if err != nil || len(values) != 1 {
		t.Fatalf("%q = %#v, %v; want one plan row", plan.Statement, values, err)
	}
}
