package postgres

import (
	"errors"
	"testing"

	pg_query "github.com/pganalyze/pg_query_go/v6"
	"google.golang.org/protobuf/reflect/protoreflect"
)

func markedTree(t *testing.T, text string) *pgTree {
	t.Helper()
	tree := mustParseTree(t, text)
	tree.Validated = true
	return tree
}

func stringConstants(t *testing.T, tree *pg_query.ParseResult) []string {
	t.Helper()
	var out []string
	err := eachMessage(tree.ProtoReflect(), func(m protoreflect.Message) error {
		if c, ok := m.Interface().(*pg_query.A_Const); ok && c.GetSval() != nil {
			out = append(out, c.GetSval().GetSval())
		}
		return nil
	})
	if err != nil {
		t.Fatalf("stringConstants: %v", err)
	}
	return out
}

func assertReason(t *testing.T, err error, want string) {
	t.Helper()
	var rejection *Rejection
	if !errors.As(err, &rejection) || rejection.Reason() != want {
		t.Fatalf("error = %v; want Rejection with reason %q", err, want)
	}
}

func hostileLexerCases() []struct{ text, rendered string } {
	return []struct{ text, rendered string }{
		{`SELECT 'a\'`, `SELECT E'a\\'`},
		{`SELECT 'a\', ' , pg_sleep(2) -- '`, `SELECT E'a\\', ' , pg_sleep(2) -- '`},
		{`SELECT 'a\', ' , pg_advisory_lock(16) -- '`, `SELECT E'a\\', ' , pg_advisory_lock(16) -- '`},
		{`SELECT E'\\', ' , pg_sleep(2) -- '`, `SELECT E'\\', ' , pg_sleep(2) -- '`},
		{`SELECT e'it\'s', E'\x41\101é'`, `SELECT 'it''s', 'AAé'`},
		{`SELECT $$a\', pg_sleep(2) -- $$, $q$it's \ $q$`, `SELECT E'a\\'', pg_sleep(2) -- ', E'it''s \\ '`},
		{`SELECT U&'\0061\005C', U&'!0041!005C' UESCAPE '!'`, `SELECT E'a\\', E'A\\'`},
		{`SELECT U&"d\0061t" FROM t`, `SELECT dat FROM t`},
		{`SELECT /* x /* nested */ ', pg_sleep(2) -- */ 'b'`, `SELECT 'b'`},
		{"SELECT 'a' -- ', pg_sleep(2)\n, 'b'", `SELECT 'a', 'b'`},
		{`SELECT 'ソ\', ' , pg_sleep(2) -- '`, `SELECT E'ソ\\', ' , pg_sleep(2) -- '`},
		{`SELECT 'it''s', 'é'`, `SELECT 'it''s', 'é'`},
	}
}

func TestDeparseIndependentOfSCS(t *testing.T) {
	for _, c := range hostileLexerCases() {
		tree := markedTree(t, c.text)
		rendered, again, err := renderAndReparse(tree.Result)
		if err != nil {
			t.Fatalf("renderAndReparse(%q): %v", c.text, err)
		}
		if rendered != c.rendered {
			t.Fatalf("renderAndReparse(%q) = %q; want %q", c.text, rendered, c.rendered)
		}
		if err := checkLexerIndependent(rendered); err != nil {
			t.Fatalf("checkLexerIndependent(%q): %v", rendered, err)
		}
		got, want := stringConstants(t, again.Result), stringConstants(t, tree.Result)
		if len(got) != len(want) {
			t.Fatalf("%q: re-parsed constants %q; want %q", c.text, got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("%q: re-parsed constants %q; want %q", c.text, got, want)
			}
		}
		if again.Validated {
			t.Fatalf("renderAndReparse(%q) returned a validated tree", c.text)
		}
	}
}

func TestRenderRejectsSCSDependentText(t *testing.T) {
	for _, text := range []string{
		`SELECT 'a\'`,
		`SELECT 'a\', 'b'`,
		`SELECT U&'d\0061t'`,
		`SELECT U&"d\0061t"`,
		`SELECT $$a$$`,
		`SELECT $q$a$q$`,
		`SELECT 1 /* c */`,
		"SELECT 1 -- c",
	} {
		assertReason(t, checkLexerIndependent(text), "render:lexer")
	}
	for _, text := range []string{
		`SELECT E'a\\'`,
		`SELECT e'a\\'`,
		`SELECT 'it''s', "a\b", b'101', x'1F'`,
		`SELECT 'é'`,
	} {
		if err := checkLexerIndependent(text); err != nil {
			t.Fatalf("checkLexerIndependent(%q) = %v; want nil", text, err)
		}
	}
}

func TestRenderNormalizesWhitespace(t *testing.T) {
	for _, c := range []struct{ text, rendered string }{
		{"select   1", "SELECT 1"},
		{"\n\tselect\t1\r\n;", "SELECT 1"},
		{"SELECT /* a /* nested */ b */ 1 -- tail", "SELECT 1"},
		{"select  a ,b from   t /* note */ where c = 'it''s' -- tail", "SELECT a, b FROM t WHERE c = 'it''s'"},
		{"SELECT CAST(x AS integer) FROM t", "SELECT x::int FROM t"},
	} {
		rendered, _, err := renderAndReparse(markedTree(t, c.text).Result)
		if err != nil {
			t.Fatalf("renderAndReparse(%q): %v", c.text, err)
		}
		if rendered != c.rendered {
			t.Fatalf("renderAndReparse(%q) = %q; want %q", c.text, rendered, c.rendered)
		}
	}
}

func TestRenderRoundTripCorpus(t *testing.T) {
	for _, text := range []string{
		"SELECT 1",
		"SELECT a, b FROM t WHERE c = 1 ORDER BY a DESC NULLS LAST LIMIT 10 OFFSET 5",
		"SELECT DISTINCT ON (a) a FROM t ORDER BY a USING <, b",
		"SELECT 1.5, -1, 1e10, 0x1F, B'101', X'1F', true, NULL",
		"SELECT x::int, x::varchar(10), a::text::bytea, '1'::\"char\"",
		"SELECT interval '1 day', timestamp '2020-01-01', date '2020-01-01'",
		"SELECT extract(year FROM now()), trim(BOTH 'x' FROM y), position('a' IN b), substring(x FROM 1 FOR 2)",
		"SELECT a FROM t WHERE b BETWEEN 1 AND 2 AND c IS DISTINCT FROM d AND e NOT IN (1, 2) AND f LIKE 'x%'",
		"SELECT a FROM t WHERE b NOT BETWEEN SYMMETRIC 2 AND 1 OR c ILIKE 'x' OR d SIMILAR TO 'y'",
		"SELECT ROW(1, 2), (1, 2), ARRAY[1, 2], x[1:2], (y).z FROM t",
		"SELECT count(*) FILTER (WHERE a > 1) OVER (PARTITION BY b ORDER BY c ROWS BETWEEN UNBOUNDED PRECEDING AND CURRENT ROW) FROM t",
		"SELECT sum(a) OVER w FROM t WINDOW w AS (PARTITION BY b)",
		"SELECT 'x' COLLATE \"C\", current_date, current_user, localtimestamp(3)",
		"SELECT CASE WHEN a THEN 1 ELSE 2 END, CASE b WHEN 1 THEN 'x' END, coalesce(a, b), nullif(a, b), greatest(a, b), least(a, b)",
		"SELECT \"SELECT\", \"a\"\"b\" FROM \"T\"",
		"SELECT a FROM t1 JOIN LATERAL (SELECT 1) s ON true NATURAL JOIN t2 LEFT JOIN t3 USING (a) CROSS JOIN t4",
		"SELECT a FROM t GROUP BY GROUPING SETS ((a), (b)), ROLLUP (a), CUBE (b) HAVING count(*) > 1",
		"WITH RECURSIVE r(n) AS (SELECT 1 UNION ALL SELECT n + 1 FROM r WHERE n < 5) SELECT * FROM r",
		"WITH a AS MATERIALIZED (SELECT 1), b AS NOT MATERIALIZED (SELECT 2) SELECT * FROM a, b",
		"SELECT x IS NULL, y IS NOT TRUE, NOT z, a AND b OR c FROM t",
		"SELECT EXISTS (SELECT 1), a = ANY (SELECT 1), a > ALL (ARRAY[1]), a IN (SELECT 1), (SELECT 1) FROM t",
		"SELECT 1 FROM generate_series(1, 3) WITH ORDINALITY AS g(x, n)",
		"SELECT 1 OPERATOR(pg_catalog.+) 2",
		"SELECT 'a' || 'b' ~~ 'c', a !~* b, -x, +y, @z FROM t",
		"TABLE t",
		"VALUES (1), (2)",
		"SELECT 1 AS x UNION SELECT 2 INTERSECT SELECT 3 EXCEPT ALL SELECT 4",
		"SELECT * FROM (SELECT 1) AS s(a), LATERAL (SELECT a) AS l",
		"SELECT a FROM t ORDER BY a FETCH FIRST 3 ROWS WITH TIES",
		"SELECT s.* FROM s.t AS s TABLESAMPLE SYSTEM (10)",
		"SELECT array_agg(a ORDER BY b), string_agg(c, ',') WITHIN GROUP (ORDER BY c) FROM t",
		"SELECT percentile_cont(0.5) WITHIN GROUP (ORDER BY a) FROM t",
		"SELECT xmlelement(name foo), json_object('a': 1), jsonb_build_object('a', 1) -> 'a'",
		"EXPLAIN SELECT 1",
		"EXPLAIN (FORMAT JSON, VERBOSE, COSTS false) SELECT a FROM t",
		"EXPLAIN VERBOSE SELECT 1",
		"SELECT table_schema AS schema, table_name AS name, table_type AS type FROM information_schema.tables WHERE table_schema NOT IN ('pg_catalog', 'information_schema') ORDER BY table_schema, table_name",
		"SELECT table_schema AS schema, table_name AS name, table_type AS type FROM information_schema.tables WHERE table_schema = 'public' ORDER BY table_schema, table_name",
		"SELECT column_name AS name, data_type AS type, is_nullable AS nullable FROM information_schema.columns WHERE table_schema = 'public' AND table_name = 't' ORDER BY ordinal_position",
		"SELECT 'it''s', E'a\\\\b', $$x$$, U&'d\\0061t'",
	} {
		tree := markedTree(t, text)
		rendered, again, err := renderAndReparse(tree.Result)
		if err != nil {
			t.Fatalf("renderAndReparse(%q): %v", text, err)
		}
		second, _, err := renderAndReparse(again.Result)
		if err != nil || second != rendered {
			t.Fatalf("render of %q is not a fixed point: %q then %q, %v", text, rendered, second, err)
		}
	}
}

func TestRenderRejectsTreeMismatch(t *testing.T) {
	nul := markedTree(t, "SELECT 'ab'")
	nul.Result.Stmts[0].Stmt.GetSelectStmt().TargetList[0].GetResTarget().Val.GetAConst().GetSval().Sval = "a\x00b"
	_, _, err := renderAndReparse(nul.Result)
	assertReason(t, err, "render:tree_mismatch")

	ignored := markedTree(t, "SELECT 1")
	other := mustParseTree(t, "SELECT 2")
	ignored.Result.Stmts[0].Stmt.GetSelectStmt().Larg = other.Result.Stmts[0].Stmt.GetSelectStmt()
	_, _, err = renderAndReparse(ignored.Result)
	assertReason(t, err, "render:tree_mismatch")

	_, err = deparseAndRevalidate(ignored)
	assertReason(t, err, "render:tree_mismatch")
}

func TestRenderRejectsInvalidUTF8(t *testing.T) {
	tree := markedTree(t, "SELECT 'ab'")
	tree.Result.Stmts[0].Stmt.GetSelectStmt().TargetList[0].GetResTarget().Val.GetAConst().GetSval().Sval = "a\xffb"
	_, _, err := renderAndReparse(tree.Result)
	assertQueryRejected(t, err)
	for _, text := range []string{"SELECT '\xff'", "SELECT 'a\xc3'", "SELECT \"\xfe\"", "SELECT '\x83\x5c'"} {
		_, err := emptyDialect().Parse(text)
		assertQueryRejected(t, err)
	}
}

func TestRenderRevalidates(t *testing.T) {
	insert := markedTree(t, "INSERT INTO t VALUES (1)")
	_, err := deparseAndRevalidate(insert)
	assertReason(t, err, "statement:InsertStmt")

	multi := markedTree(t, "SELECT 1; SELECT 2")
	_, err = deparseAndRevalidate(multi)
	assertReason(t, err, "statement_count:2")

	for _, text := range []string{"select   1", "SELECT 'a\\', ' , pg_sleep(2) -- '", "SELECT a FROM t"} {
		tree := markedTree(t, text)
		rendered, _, err := renderAndReparse(tree.Result)
		if err != nil {
			t.Fatalf("renderAndReparse(%q): %v", text, err)
		}
		want := validateStaticTree(mustParseTree(t, rendered))
		got, err := deparseAndRevalidate(tree)
		if (err == nil) != (want == nil) || (err != nil && err.Error() != want.Error()) {
			t.Fatalf("deparseAndRevalidate(%q) error = %v; want the static result of the rendered text %v", text, err, want)
		}
		if err == nil && got != rendered {
			t.Fatalf("deparseAndRevalidate(%q) = %q; want %q", text, got, rendered)
		}
		if !tree.Validated {
			t.Fatalf("deparseAndRevalidate(%q) changed the original tree", text)
		}
	}
}

func TestRenderRequiresValidatedTree(t *testing.T) {
	for _, tree := range []*pgTree{nil, {}, mustParseTree(t, "SELECT 1")} {
		_, err := deparseAndRevalidate(tree)
		assertQueryRejected(t, err)
	}
}
