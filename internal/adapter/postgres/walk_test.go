package postgres

import (
	"errors"
	"testing"

	pg_query "github.com/pganalyze/pg_query_go/v6"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
)

const (
	listTablesAllText    = "SELECT table_schema AS schema, table_name AS name, table_type AS type FROM information_schema.tables WHERE table_schema NOT IN ('pg_catalog', 'information_schema') ORDER BY table_schema, table_name"
	listTablesSchemaText = "SELECT table_schema AS schema, table_name AS name, table_type AS type FROM information_schema.tables WHERE table_schema = 'sales' ORDER BY table_schema, table_name"
	describeDefaultText  = "SELECT column_name AS name, data_type AS type, is_nullable AS nullable FROM information_schema.columns WHERE table_schema = 'public' AND table_name = 'orders' ORDER BY ordinal_position"
	describeSchemaText   = "SELECT column_name AS name, data_type AS type, is_nullable AS nullable FROM information_schema.columns WHERE table_schema = 'sales' AND table_name = 'it''s' ORDER BY ordinal_position"
)

var readCorpus = []string{
	listTablesAllText,
	listTablesSchemaText,
	describeDefaultText,
	describeSchemaText,
	"SELECT 1",
	"SELECT * FROM orders",
	"SELECT o.id, o.total FROM public.orders AS o WHERE o.total > 100.50 ORDER BY o.total DESC LIMIT 10 OFFSET 20",
	"SELECT DISTINCT customer_id FROM orders",
	"SELECT DISTINCT ON (customer_id) customer_id, created_at FROM orders ORDER BY customer_id, created_at DESC",
	"SELECT c.name, count(*) AS n, sum(o.total) AS revenue, avg(o.total)::numeric(12,2) AS aov FROM customers c JOIN orders o ON o.customer_id = c.id LEFT JOIN refunds r ON r.order_id = o.id WHERE o.created_at >= date '2024-01-01' AND r.id IS NULL GROUP BY c.name HAVING count(*) > 5 ORDER BY revenue DESC NULLS LAST",
	"SELECT region, product, sum(amount) FROM sales GROUP BY ROLLUP (region, product)",
	"SELECT region, sum(amount) FROM sales GROUP BY GROUPING SETS ((region), (product), ())",
	"SELECT region FROM sales GROUP BY CUBE (region, product), ()",
	"SELECT region FROM sales GROUP BY DISTINCT region",
	"SELECT id, row_number() OVER (PARTITION BY customer_id ORDER BY created_at) AS rn, lag(total) OVER w, sum(total) OVER (ORDER BY created_at ROWS BETWEEN 6 PRECEDING AND CURRENT ROW) FROM orders WINDOW w AS (PARTITION BY customer_id ORDER BY created_at)",
	"SELECT sum(total) OVER (w RANGE BETWEEN UNBOUNDED PRECEDING AND UNBOUNDED FOLLOWING EXCLUDE TIES) FROM orders WINDOW w AS (ORDER BY created_at)",
	"SELECT CASE WHEN total > 1000 THEN 'large' WHEN total > 100 THEN 'medium' ELSE 'small' END AS bucket, CASE status WHEN 'a' THEN 1 WHEN 'b' THEN 2 END FROM orders",
	"SELECT COALESCE(nickname, first_name, 'unknown'), NULLIF(discount, 0), GREATEST(a, b, c), LEAST(a, b) FROM customers",
	"SELECT id::text, CAST(total AS bigint), total::numeric(10, 2), tags::text[], name::varchar(20) FROM orders",
	"SELECT * FROM orders WHERE status IN ('paid', 'shipped') AND region NOT IN ('eu', 'us') AND id = ANY (ARRAY[1, 2, 3]) AND id <> ALL ('{4,5}'::int[])",
	"SELECT * FROM customers WHERE email LIKE '%@example.com' OR name ILIKE 'a%' OR name NOT LIKE 'b%' OR code SIMILAR TO 'x%' OR name ~ '^a' OR name !~* 'z'",
	"SELECT now() - interval '7 days', current_date + 1, date_trunc('month', created_at), extract(year FROM created_at), created_at AT TIME ZONE 'UTC', age(now(), created_at), interval '1' day FROM orders",
	"SELECT CURRENT_DATE, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP(3), LOCALTIME, LOCALTIMESTAMP, CURRENT_USER, SESSION_USER, CURRENT_SCHEMA, CURRENT_CATALOG, USER",
	"SELECT payload -> 'a', payload ->> 'b', payload #> '{a,b}', payload #>> '{a,b}', payload @> '{\"k\": 1}'::jsonb, payload ? 'k', jsonb_array_length(payload -> 'items') FROM events",
	"SELECT jsonb_agg(x ORDER BY x), json_build_object('a', 1), to_jsonb(t) FROM t",
	"SELECT string_agg(DISTINCT name, ', ' ORDER BY name), array_agg(id), count(DISTINCT customer_id), count(*) FILTER (WHERE total > 10) FROM orders",
	"SELECT percentile_cont(0.5) WITHIN GROUP (ORDER BY total), mode() WITHIN GROUP (ORDER BY status) FROM orders",
	"WITH recent AS (SELECT * FROM orders WHERE created_at > now() - interval '30 days'), top AS MATERIALIZED (SELECT customer_id, sum(total) AS s FROM recent GROUP BY customer_id) SELECT * FROM top WHERE s > (SELECT avg(s) FROM top)",
	"WITH RECURSIVE tree(id, parent_id, depth) AS (SELECT id, parent_id, 0 FROM nodes WHERE parent_id IS NULL UNION ALL SELECT n.id, n.parent_id, t.depth + 1 FROM nodes n JOIN tree t ON n.parent_id = t.id) SELECT * FROM tree",
	"WITH x AS NOT MATERIALIZED (SELECT 1 AS a) SELECT a FROM x",
	"SELECT id FROM a UNION SELECT id FROM b INTERSECT SELECT id FROM c EXCEPT ALL SELECT id FROM d",
	"(SELECT id FROM a ORDER BY id LIMIT 1) UNION ALL (SELECT id FROM b)",
	"SELECT * FROM orders o WHERE EXISTS (SELECT 1 FROM items i WHERE i.order_id = o.id) AND NOT EXISTS (SELECT 1 FROM refunds r WHERE r.order_id = o.id)",
	"SELECT id, (SELECT max(total) FROM orders o WHERE o.customer_id = c.id) AS max_total FROM customers c WHERE c.id IN (SELECT customer_id FROM orders) AND c.score > ALL (SELECT score FROM bans)",
	"SELECT * FROM customers c, LATERAL (SELECT * FROM orders o WHERE o.customer_id = c.id ORDER BY created_at DESC LIMIT 3) recent",
	"SELECT * FROM customers c CROSS JOIN LATERAL jsonb_array_elements(c.tags) AS tag(value)",
	"SELECT * FROM a NATURAL JOIN b FULL OUTER JOIN c USING (id) RIGHT JOIN d ON d.id = c.id CROSS JOIN e",
	"SELECT * FROM a JOIN b USING (id) AS j WHERE j.id > 1",
	"SELECT * FROM generate_series(1, 10) AS g(n)",
	"SELECT * FROM unnest(ARRAY[1, 2], ARRAY['a', 'b']) WITH ORDINALITY AS u(n, s, ord)",
	"SELECT * FROM ROWS FROM (generate_series(1, 3), unnest(ARRAY[4, 5])) AS r(a, b)",
	"VALUES (1, 'one'), (2, 'two')",
	"SELECT * FROM (VALUES (1, 'a'), (2, 'b')) AS v(id, label)",
	"TABLE orders",
	"SELECT * FROM ONLY parent_table",
	"SELECT a.* , b.id FROM a, b",
	"SELECT arr[1], arr[1:2], arr[:2], arr[2:], (rec).field, (rec).*, (ARRAY[1, 2, 3])[2] FROM t",
	"SELECT ROW(1, 'a'), (1, 2) < (3, 4), ARRAY[ARRAY[1, 2], ARRAY[3, 4]], ARRAY(SELECT id FROM t)",
	"SELECT name COLLATE \"C\" FROM t ORDER BY name COLLATE \"C\"",
	"SELECT a IS NULL, a IS NOT NULL, b IS TRUE, b IS NOT FALSE, b IS UNKNOWN, a IS DISTINCT FROM b, a IS NOT DISTINCT FROM b FROM t",
	"SELECT -1, +2, NOT true, 2 ^ 3, 7 % 2, 'a' || 'b', 1.5e10, 0x1F, TRUE, FALSE, NULL",
	"SELECT a FROM t WHERE a BETWEEN 1 AND 10 AND b NOT BETWEEN SYMMETRIC 5 AND 1",
	"SELECT id FROM t ORDER BY id USING <, name DESC NULLS FIRST",
	"SELECT id FROM t ORDER BY id FETCH FIRST 5 ROWS WITH TIES",
	"SELECT id FROM t LIMIT ALL",
	"SELECT 1 OPERATOR(pg_catalog.+) 2",
	"SELECT pg_catalog.lower(name) FROM t",
	"SELECT format('%s-%s', a, b), concat_ws(',', a, b), substring(name FROM 2 FOR 3), position('a' IN name), trim(BOTH ' ' FROM name), overlay(name PLACING 'x' FROM 1 FOR 1) FROM t",
	"SELECT round(avg(total), 2), floor(x), ceil(x), abs(x), coalesce(sum(x), 0) / NULLIF(count(*), 0) FROM t",
	"SELECT to_char(created_at, 'YYYY-MM'), make_date(2024, 1, 1), created_at::date - '2024-01-01'::date FROM t",
	"SELECT array_length(tags, 1), cardinality(tags), 'x' = ANY (tags) FROM t",
	"SELECT EXISTS (SELECT 1)",
	"SELECT count(*) FROM t WHERE created_at >= now() - '1 hour'::interval",
	"SELECT random(), gen_random_uuid(), clock_timestamp()",
	"SELECT x FROM t WHERE (a, b) IN (SELECT a, b FROM u)",
	"EXPLAIN SELECT * FROM orders",
	"EXPLAIN (FORMAT JSON, VERBOSE, COSTS false) SELECT * FROM orders WHERE id = 1",
	"EXPLAIN VERBOSE SELECT 1",
	"EXPLAIN (COSTS off, FORMAT yaml) WITH x AS (SELECT 1) SELECT * FROM x",
}

func walkReason(t *testing.T, tree *pgTree) string {
	t.Helper()
	err := walkAllowlist(tree)
	if err == nil {
		return ""
	}
	var rejection *Rejection
	if !errors.As(err, &rejection) {
		t.Fatalf("walkAllowlist() = %T %v; want *Rejection", err, err)
	}
	return rejection.Reason()
}

func firstSelect(t *testing.T, tree *pgTree) *pg_query.SelectStmt {
	t.Helper()
	sel := tree.Result.GetStmts()[0].GetStmt().GetSelectStmt()
	if sel == nil {
		t.Fatal("first statement is not a SelectStmt")
	}
	return sel
}

func TestReadCorpusAccepted(t *testing.T) {
	if len(readCorpus) < 40 {
		t.Fatalf("read corpus has %d statements; want 40 or more", len(readCorpus))
	}
	for _, text := range readCorpus {
		if reason := walkReason(t, mustParseTree(t, text)); reason != "" {
			t.Errorf("walkAllowlist(%q) rejected: %s", text, reason)
		}
	}
}

func TestReadCorpusAcceptedAfterRender(t *testing.T) {
	for _, text := range readCorpus {
		rendered, err := render(mustParseTree(t, text).Result)
		if err != nil {
			t.Errorf("render(%q): %v", text, err)
			continue
		}
		if reason := walkReason(t, mustParseTree(t, rendered)); reason != "" {
			t.Errorf("walkAllowlist(render(%q)) = %q rejected: %s", text, rendered, reason)
		}
	}
}

func TestAC14_FieldOutsideAllowlist(t *testing.T) {
	cases := []struct {
		name   string
		text   string
		mutate func(t *testing.T, tree *pgTree)
		want   string
	}{
		{"into_clause", "SELECT a FROM t", func(t *testing.T, tree *pgTree) {
			firstSelect(t, tree).IntoClause = &pg_query.IntoClause{Rel: &pg_query.RangeVar{Relname: "t2", Inh: true, Relpersistence: "p"}}
		}, "SelectStmt.into_clause"},
		{"catalogname", "SELECT a FROM t", func(t *testing.T, tree *pgTree) {
			firstSelect(t, tree).GetFromClause()[0].GetRangeVar().Catalogname = "otherdb"
		}, "RangeVar.catalogname"},
		{"restarget indirection", "SELECT a FROM t", func(t *testing.T, tree *pgTree) {
			firstSelect(t, tree).GetTargetList()[0].GetResTarget().Indirection = []*pg_query.Node{pg_query.MakeStrNode("x")}
		}, "ResTarget.indirection"},
		{"typename setof", "SELECT a::int FROM t", func(t *testing.T, tree *pgTree) {
			firstSelect(t, tree).GetTargetList()[0].GetResTarget().GetVal().GetTypeCast().GetTypeName().Setof = true
		}, "TypeName.setof"},
		{"nulltest argisrow", "SELECT a IS NULL FROM t", func(t *testing.T, tree *pgTree) {
			firstSelect(t, tree).GetTargetList()[0].GetResTarget().GetVal().GetNullTest().Argisrow = true
		}, "NullTest.argisrow"},
		{"joinexpr rtindex", "SELECT 1 FROM a JOIN b ON true", func(t *testing.T, tree *pgTree) {
			firstSelect(t, tree).GetFromClause()[0].GetJoinExpr().Rtindex = 3
		}, "JoinExpr.rtindex"},
		{"cte refcount", "WITH c AS (SELECT 1) SELECT * FROM c", func(t *testing.T, tree *pgTree) {
			firstSelect(t, tree).GetWithClause().GetCtes()[0].GetCommonTableExpr().Cterefcount = 1
		}, "CommonTableExpr.cterefcount"},
		{"boolexpr xpr", "SELECT a AND b FROM t", func(t *testing.T, tree *pgTree) {
			firstSelect(t, tree).GetTargetList()[0].GetResTarget().GetVal().GetBoolExpr().Xpr = pg_query.MakeAConstIntNode(1, 0)
		}, "BoolExpr.xpr"},
		{"defelem defnamespace", "EXPLAIN (VERBOSE) SELECT 1", func(t *testing.T, tree *pgTree) {
			tree.Result.GetStmts()[0].GetStmt().GetExplainStmt().GetOptions()[0].GetDefElem().Defnamespace = "x"
		}, "DefElem.defnamespace"},
		{"rangefunction coldeflist", "SELECT * FROM f() AS x", func(t *testing.T, tree *pgTree) {
			firstSelect(t, tree).GetFromClause()[0].GetRangeFunction().Coldeflist = []*pg_query.Node{pg_query.MakeStrNode("a")}
		}, "RangeFunction.coldeflist"},
		{"sublink id", "SELECT (SELECT 1)", func(t *testing.T, tree *pgTree) {
			firstSelect(t, tree).GetTargetList()[0].GetResTarget().GetVal().GetSubLink().SubLinkId = 1
		}, "SubLink.sub_link_id"},
		{"deep field in CTE subquery", "WITH c AS (SELECT * FROM (SELECT x IS NULL FROM t) s) SELECT * FROM c", func(t *testing.T, tree *pgTree) {
			inner := firstSelect(t, tree).GetWithClause().GetCtes()[0].GetCommonTableExpr().GetCtequery().GetSelectStmt()
			sub := inner.GetFromClause()[0].GetRangeSubselect().GetSubquery().GetSelectStmt()
			sub.GetTargetList()[0].GetResTarget().GetVal().GetNullTest().Argisrow = true
		}, "NullTest.argisrow"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tree := mustParseTree(t, tc.text)
			if reason := walkReason(t, tree); reason != "" {
				t.Fatalf("walkAllowlist(%q) before the change rejected: %s", tc.text, reason)
			}
			tc.mutate(t, tree)
			if got := walkReason(t, tree); got != tc.want {
				t.Fatalf("walkAllowlist(changed %q) reason = %q; want %q", tc.text, got, tc.want)
			}
		})
	}
}

func TestAC14_EveryExcludedFieldRejects(t *testing.T) {
	checked := 0
	for typeName, fields := range allowedNodeFields {
		mt, err := protoregistry.GlobalTypes.FindMessageByName(protoreflect.FullName(pgQueryPackage + "." + typeName))
		if err != nil {
			t.Fatalf("type %s: %v", typeName, err)
		}
		allowed := map[string]bool{}
		for _, f := range fields {
			allowed[f] = true
		}
		desc := mt.Descriptor()
		for i := 0; i < desc.Fields().Len(); i++ {
			fd := desc.Fields().Get(i)
			if allowed[string(fd.Name())] {
				continue
			}
			m := mt.New()
			setNonZero(t, m, fd)
			root := wrapInSelect(t, m.Interface())
			want := typeName + "." + string(fd.Name())
			if got := walkReason(t, &pgTree{Result: root}); got != want {
				t.Errorf("walkAllowlist(%s with %s set) reason = %q; want %q", typeName, fd.Name(), got, want)
			}
			checked++
		}
	}
	if checked == 0 {
		t.Fatal("no excluded field checked")
	}
}

func setNonZero(t *testing.T, m protoreflect.Message, fd protoreflect.FieldDescriptor) {
	t.Helper()
	switch {
	case fd.IsList() && fd.Message() != nil:
		m.Mutable(fd).List().Append(protoreflect.ValueOfMessage(pg_query.MakeStrNode("x").ProtoReflect()))
	case fd.Message() != nil && fd.Message().FullName() == nodeDescriptor.FullName():
		m.Set(fd, protoreflect.ValueOfMessage(pg_query.MakeStrNode("x").ProtoReflect()))
	case fd.Message() != nil:
		m.Set(fd, protoreflect.ValueOfMessage(m.NewField(fd).Message()))
	case fd.Kind() == protoreflect.BoolKind:
		m.Set(fd, protoreflect.ValueOfBool(true))
	case fd.Kind() == protoreflect.StringKind:
		m.Set(fd, protoreflect.ValueOfString("x"))
	case fd.Kind() == protoreflect.Int32Kind:
		m.Set(fd, protoreflect.ValueOfInt32(1))
	case fd.Kind() == protoreflect.Uint32Kind:
		m.Set(fd, protoreflect.ValueOfUint32(1))
	case fd.Kind() == protoreflect.EnumKind:
		m.Set(fd, protoreflect.ValueOfEnum(1))
	default:
		t.Fatalf("field %s has kind %s; add it to setNonZero", fd.FullName(), fd.Kind())
	}
	if !m.Has(fd) {
		t.Fatalf("field %s is not populated after set", fd.FullName())
	}
}

func wrapInSelect(t *testing.T, msg proto.Message) *pg_query.ParseResult {
	t.Helper()
	switch v := msg.(type) {
	case *pg_query.ParseResult:
		v.Stmts = []*pg_query.RawStmt{{Stmt: &pg_query.Node{Node: &pg_query.Node_SelectStmt{SelectStmt: &pg_query.SelectStmt{}}}}}
		return v
	case *pg_query.RawStmt:
		return &pg_query.ParseResult{Stmts: []*pg_query.RawStmt{v}}
	case *pg_query.ExplainStmt:
		return &pg_query.ParseResult{Stmts: []*pg_query.RawStmt{{Stmt: &pg_query.Node{Node: &pg_query.Node_ExplainStmt{ExplainStmt: v}}}}}
	case *pg_query.DefElem:
		explain := &pg_query.ExplainStmt{Options: []*pg_query.Node{{Node: &pg_query.Node_DefElem{DefElem: v}}}}
		return &pg_query.ParseResult{Stmts: []*pg_query.RawStmt{{Stmt: &pg_query.Node{Node: &pg_query.Node_ExplainStmt{ExplainStmt: explain}}}}}
	}
	node := &pg_query.Node{}
	nodeMsg := node.ProtoReflect()
	for i := 0; i < nodeDescriptor.Fields().Len(); i++ {
		fd := nodeDescriptor.Fields().Get(i)
		if fd.Message().FullName() == msg.ProtoReflect().Descriptor().FullName() {
			nodeMsg.Set(fd, protoreflect.ValueOfMessage(msg.ProtoReflect()))
		}
	}
	if nodeMsg.WhichOneof(nodeDescriptor.Oneofs().ByName(nodeOneofName)) == nil {
		t.Fatalf("type %T is not a member of Node", msg)
	}
	sel := &pg_query.SelectStmt{TargetList: []*pg_query.Node{node}}
	return &pg_query.ParseResult{Stmts: []*pg_query.RawStmt{{Stmt: &pg_query.Node{Node: &pg_query.Node_SelectStmt{SelectStmt: sel}}}}}
}

func TestUnknownNodeRejects(t *testing.T) {
	cases := []struct {
		text string
		want string
	}{
		{"INSERT INTO t VALUES (1)", "node:InsertStmt"},
		{"UPDATE t SET a = 1", "node:UpdateStmt"},
		{"DELETE FROM t", "node:DeleteStmt"},
		{"MERGE INTO t USING s ON t.id = s.id WHEN MATCHED THEN DELETE", "node:MergeStmt"},
		{"TRUNCATE t", "node:TruncateStmt"},
		{"DROP TABLE t", "node:DropStmt"},
		{"CREATE TABLE t (a int)", "node:CreateStmt"},
		{"CREATE TEMPORARY TABLE t AS SELECT 1", "node:CreateTableAsStmt"},
		{"CREATE VIEW v AS SELECT 1", "node:ViewStmt"},
		{"ALTER TABLE t ADD COLUMN b int", "node:AlterTableStmt"},
		{"ALTER SYSTEM SET work_mem = '1GB'", "node:AlterSystemStmt"},
		{"GRANT SELECT ON t TO PUBLIC", "node:GrantStmt"},
		{"COPY t TO STDOUT", "node:CopyStmt"},
		{"COPY (SELECT 1) TO PROGRAM 'touch /tmp/x'", "node:CopyStmt"},
		{"COPY (SELECT 1) TO '/tmp/x'", "node:CopyStmt"},
		{"SET search_path = evil", "node:VariableSetStmt"},
		{"SET ROLE postgres", "node:VariableSetStmt"},
		{"SET SESSION AUTHORIZATION postgres", "node:VariableSetStmt"},
		{"RESET ALL", "node:VariableSetStmt"},
		{"SHOW ALL", "node:VariableShowStmt"},
		{"DISCARD ALL", "node:DiscardStmt"},
		{"PREPARE p AS SELECT 1", "node:PrepareStmt"},
		{"EXECUTE p", "node:ExecuteStmt"},
		{"DEALLOCATE ALL", "node:DeallocateStmt"},
		{"DO $$ BEGIN END $$", "node:DoStmt"},
		{"CALL p()", "node:CallStmt"},
		{"LISTEN c", "node:ListenStmt"},
		{"NOTIFY c, 'x'", "node:NotifyStmt"},
		{"UNLISTEN *", "node:UnlistenStmt"},
		{"LOCK TABLE t IN ACCESS EXCLUSIVE MODE", "node:LockStmt"},
		{"CHECKPOINT", "node:CheckPointStmt"},
		{"PREPARE TRANSACTION 'x'", "node:TransactionStmt"},
		{"COMMIT PREPARED 'x'", "node:TransactionStmt"},
		{"BEGIN", "node:TransactionStmt"},
		{"COMMIT", "node:TransactionStmt"},
		{"SAVEPOINT s", "node:TransactionStmt"},
		{"VACUUM t", "node:VacuumStmt"},
		{"ANALYZE t", "node:VacuumStmt"},
		{"LOAD 'x'", "node:LoadStmt"},
		{"DECLARE c CURSOR FOR SELECT 1", "node:DeclareCursorStmt"},
		{"FETCH ALL FROM c", "node:FetchStmt"},
		{"CLOSE c", "node:ClosePortalStmt"},
		{"REFRESH MATERIALIZED VIEW m", "node:RefreshMatViewStmt"},
		{"REINDEX TABLE t", "node:ReindexStmt"},
		{"CLUSTER t", "node:ClusterStmt"},
		{"COMMENT ON TABLE t IS 'x'", "node:CommentStmt"},
		{"CREATE EXTENSION dblink", "node:CreateExtensionStmt"},
		{"CREATE FUNCTION f() RETURNS int LANGUAGE sql AS 'SELECT 1'", "node:CreateFunctionStmt"},
		{"EXPLAIN INSERT INTO t VALUES (1)", "node:InsertStmt"},
		{"EXPLAIN ANALYZE DELETE FROM t", "node:DeleteStmt"},
		{"WITH d AS (DELETE FROM t RETURNING *) SELECT * FROM d", "node:DeleteStmt"},
		{"WITH u AS (UPDATE t SET a = 1 RETURNING *) SELECT * FROM u", "node:UpdateStmt"},
		{"WITH i AS (INSERT INTO t VALUES (1) RETURNING *) SELECT * FROM i", "node:InsertStmt"},
		{"SELECT * INTO t2 FROM t", "SelectStmt.into_clause"},
		{"SELECT * FROM t FOR UPDATE", "SelectStmt.locking_clause"},
		{"SELECT * FROM t FOR NO KEY UPDATE", "SelectStmt.locking_clause"},
		{"SELECT * FROM t FOR SHARE", "SelectStmt.locking_clause"},
		{"SELECT * FROM t FOR KEY SHARE SKIP LOCKED", "SelectStmt.locking_clause"},
		{"SELECT $1", "node:ParamRef"},
		{"SELECT B'101'", "A_Const.bsval"},
		{"SELECT X'1F'", "A_Const.bsval"},
		{"SELECT * FROM t TABLESAMPLE SYSTEM (1)", "node:RangeTableSample"},
		{"SELECT GROUPING(a) FROM t GROUP BY a", "node:GroupingFunc"},
		{"SELECT xmlelement(name a)", "node:XmlExpr"},
		{"SELECT xmlserialize(document x AS text) FROM t", "node:XmlSerialize"},
		{"SELECT * FROM XMLTABLE('/a' PASSING x COLUMNS a int) AS t", "node:RangeTableFunc"},
		{"SELECT JSON_OBJECT('a': 1)", "node:JsonObjectConstructor"},
		{"SELECT x IS JSON FROM t", "node:JsonIsPredicate"},
		{"SELECT * FROM JSON_TABLE('[]'::jsonb, '$[*]' COLUMNS (a int PATH '$')) AS j", "node:JsonTable"},
		{"SELECT * FROM f() AS x(a int)", "RangeFunction.coldeflist"},
		{"WITH RECURSIVE r(n) AS (SELECT 1 UNION ALL SELECT n + 1 FROM r) SEARCH DEPTH FIRST BY n SET o SELECT * FROM r", "CommonTableExpr.search_clause"},
		{"WITH RECURSIVE r(n) AS (SELECT 1 UNION ALL SELECT n + 1 FROM r) CYCLE n SET c USING p SELECT * FROM r", "CommonTableExpr.cycle_clause"},
		{"SELECT 1 FROM t WHERE a = (SELECT b FROM u TABLESAMPLE BERNOULLI (5))", "node:RangeTableSample"},
	}
	for _, tc := range cases {
		t.Run(tc.text, func(t *testing.T) {
			if got := walkReason(t, mustParseTree(t, tc.text)); got != tc.want {
				t.Fatalf("walkAllowlist(%q) reason = %q; want %q", tc.text, got, tc.want)
			}
		})
	}
}

func TestUnknownNodeRejectsHandBuilt(t *testing.T) {
	t.Run("DefElem outside ExplainStmt", func(t *testing.T) {
		tree := mustParseTree(t, "SELECT a FROM t")
		firstSelect(t, tree).GetTargetList()[0].GetResTarget().Val = &pg_query.Node{Node: &pg_query.Node_DefElem{DefElem: &pg_query.DefElem{Defname: "analyze"}}}
		if got := walkReason(t, tree); got != "node:DefElem" {
			t.Fatalf("reason = %q; want node:DefElem", got)
		}
	})
	t.Run("empty Node outside DISTINCT and function list", func(t *testing.T) {
		tree := mustParseTree(t, "SELECT a FROM t")
		firstSelect(t, tree).GetTargetList()[0].GetResTarget().Val = &pg_query.Node{}
		if got := walkReason(t, tree); got != "Node.node" {
			t.Fatalf("reason = %q; want Node.node", got)
		}
	})
	t.Run("nil Node in a list", func(t *testing.T) {
		tree := mustParseTree(t, "SELECT a FROM t")
		sel := firstSelect(t, tree)
		sel.TargetList = append(sel.TargetList, nil)
		if got := walkReason(t, tree); got != "Node.node" {
			t.Fatalf("reason = %q; want Node.node", got)
		}
	})
	t.Run("empty Node in DISTINCT with ON list", func(t *testing.T) {
		tree := mustParseTree(t, "SELECT DISTINCT ON (a) a FROM t")
		sel := firstSelect(t, tree)
		sel.DistinctClause = append(sel.DistinctClause, &pg_query.Node{})
		if got := walkReason(t, tree); got != "Node.node" {
			t.Fatalf("reason = %q; want Node.node", got)
		}
	})
	t.Run("empty function in function list", func(t *testing.T) {
		tree := mustParseTree(t, "SELECT * FROM generate_series(1, 2) g")
		items := firstSelect(t, tree).GetFromClause()[0].GetRangeFunction().GetFunctions()[0].GetList().GetItems()
		items[0] = &pg_query.Node{}
		if got := walkReason(t, tree); got != "Node.node" {
			t.Fatalf("reason = %q; want Node.node", got)
		}
	})
	t.Run("unknown protobuf field", func(t *testing.T) {
		tree := mustParseTree(t, "SELECT a FROM t")
		rv := firstSelect(t, tree).GetFromClause()[0].GetRangeVar()
		rv.ProtoReflect().SetUnknown(protoreflect.RawFields{0xF8, 0x07, 0x01})
		if got := walkReason(t, tree); got != "RangeVar.unknown_fields" {
			t.Fatalf("reason = %q; want RangeVar.unknown_fields", got)
		}
	})
	t.Run("unknown statement in the result", func(t *testing.T) {
		tree := mustParseTree(t, "SELECT 1")
		tree.Result.Stmts[0].Stmt = &pg_query.Node{Node: &pg_query.Node_VariableSetStmt{VariableSetStmt: &pg_query.VariableSetStmt{Name: "role"}}}
		if got := walkReason(t, tree); got != "node:VariableSetStmt" {
			t.Fatalf("reason = %q; want node:VariableSetStmt", got)
		}
	})
	t.Run("nil tree", func(t *testing.T) {
		if got := walkReason(t, &pgTree{}); got != "node:ParseResult" {
			t.Fatalf("reason = %q; want node:ParseResult", got)
		}
		if got := walkReason(t, nil); got != "node:ParseResult" {
			t.Fatalf("reason = %q; want node:ParseResult", got)
		}
	})
}

func TestVisitsSubqueriesCTEsSetOpsLateral(t *testing.T) {
	cases := []struct {
		text string
		want string
	}{
		{"WITH c AS (SELECT $1) SELECT * FROM c", "node:ParamRef"},
		{"WITH a AS (SELECT 1), b AS (WITH c AS (SELECT $1) SELECT * FROM c) SELECT * FROM b", "node:ParamRef"},
		{"SELECT * FROM (SELECT $1) s", "node:ParamRef"},
		{"SELECT * FROM t WHERE a IN (SELECT $1)", "node:ParamRef"},
		{"SELECT (SELECT (SELECT (SELECT $1)))", "node:ParamRef"},
		{"SELECT 1 UNION SELECT $1", "node:ParamRef"},
		{"SELECT 1 UNION ALL (SELECT 2 EXCEPT SELECT $1)", "node:ParamRef"},
		{"SELECT * FROM t, LATERAL (SELECT $1) l", "node:ParamRef"},
		{"SELECT * FROM t CROSS JOIN LATERAL f($1) l", "node:ParamRef"},
		{"SELECT * FROM t JOIN u ON u.a = $1", "node:ParamRef"},
		{"SELECT * FROM t JOIN (u JOIN v ON v.a = $1) ON true", "node:ParamRef"},
		{"SELECT sum(a) OVER (PARTITION BY $1) FROM t", "node:ParamRef"},
		{"SELECT sum(a) OVER (ORDER BY a ROWS BETWEEN $1 PRECEDING AND CURRENT ROW) FROM t", "node:ParamRef"},
		{"SELECT sum(a) OVER w FROM t WINDOW w AS (ORDER BY $1)", "node:ParamRef"},
		{"SELECT a FROM t ORDER BY $1", "node:ParamRef"},
		{"SELECT f(g(h($1)))", "node:ParamRef"},
		{"SELECT count(*) FILTER (WHERE a = $1) FROM t", "node:ParamRef"},
		{"SELECT string_agg(a, ',' ORDER BY $1) FROM t", "node:ParamRef"},
		{"SELECT CASE WHEN true THEN 1 ELSE $1 END", "node:ParamRef"},
		{"SELECT COALESCE(a, $1) FROM t", "node:ParamRef"},
		{"SELECT a FROM t GROUP BY a HAVING count(*) > $1", "node:ParamRef"},
		{"SELECT a FROM t GROUP BY GROUPING SETS ((a), ($1))", "node:ParamRef"},
		{"SELECT a FROM t LIMIT $1", "node:ParamRef"},
		{"SELECT a FROM t OFFSET $1", "node:ParamRef"},
		{"VALUES (1), ($1)", "node:ParamRef"},
		{"SELECT * FROM (VALUES (1), ($1)) v", "node:ParamRef"},
		{"EXPLAIN SELECT $1", "node:ParamRef"},
		{"SELECT ARRAY[1, $1]", "node:ParamRef"},
		{"SELECT (ARRAY[1])[$1]", "node:ParamRef"},
		{"SELECT ROW(1, $1)", "node:ParamRef"},
		{"SELECT $1::int", "node:ParamRef"},
		{"SELECT a COLLATE \"C\" FROM t WHERE a = ANY (ARRAY[$1])", "node:ParamRef"},
		{"SELECT * FROM generate_series(1, $1)", "node:ParamRef"},
		{"SELECT * FROM ROWS FROM (generate_series(1, 2), f($1)) r", "node:ParamRef"},
		{"SELECT EXISTS (SELECT 1 FROM t WHERE a = $1)", "node:ParamRef"},
		{"SELECT a FROM t WHERE a BETWEEN 1 AND $1", "node:ParamRef"},
		{"SELECT a IS DISTINCT FROM $1 FROM t", "node:ParamRef"},
		{"SELECT GREATEST(1, $1)", "node:ParamRef"},
		{"SELECT * FROM t WHERE EXISTS (SELECT * FROM u FOR SHARE)", "SelectStmt.locking_clause"},
		{"WITH c AS (SELECT * FROM t FOR UPDATE) SELECT * FROM c", "SelectStmt.locking_clause"},
		{"SELECT 1 UNION (SELECT 1 FROM t FOR UPDATE)", "SelectStmt.locking_clause"},
		{"SELECT * FROM t, LATERAL (SELECT * FROM u FOR KEY SHARE) l", "SelectStmt.locking_clause"},
		{"WITH c AS (SELECT 1) SELECT * FROM (SELECT B'1') s", "A_Const.bsval"},
		{"SELECT * FROM t WHERE a = (SELECT b FROM u TABLESAMPLE SYSTEM (1))", "node:RangeTableSample"},
		{"WITH x AS (SELECT 1) SELECT * FROM x WHERE EXISTS (WITH d AS (DELETE FROM t RETURNING *) SELECT 1 FROM d)", "node:DeleteStmt"},
	}
	for _, tc := range cases {
		t.Run(tc.text, func(t *testing.T) {
			if got := walkReason(t, mustParseTree(t, tc.text)); got != tc.want {
				t.Fatalf("walkAllowlist(%q) reason = %q; want %q", tc.text, got, tc.want)
			}
		})
	}
}
