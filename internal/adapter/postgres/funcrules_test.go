package postgres

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

func assertFunctionRulesReason(t *testing.T, text, want string) {
	t.Helper()
	err := checkFunctionNames(mustParseTree(t, text))
	var rejection *Rejection
	if !errors.As(err, &rejection) {
		t.Fatalf("checkFunctionNames(%q) = %v; want a Rejection %q", text, err, want)
	}
	if got := rejection.Reason(); got != want {
		t.Fatalf("checkFunctionNames(%q) reason = %q; want %q", text, got, want)
	}
}

func assertFunctionRulesAccepted(t *testing.T, text string) *pgTree {
	t.Helper()
	tree := mustParseTree(t, text)
	if err := checkFunctionNames(tree); err != nil {
		t.Fatalf("checkFunctionNames(%q) = %v; want nil", text, err)
	}
	return tree
}

func sortedNames(names []qualifiedName) []string {
	out := make([]string, 0, len(names))
	for _, n := range names {
		out = append(out, n.String())
	}
	slices.Sort(out)
	return out
}

func assertNames(t *testing.T, what string, got []qualifiedName, want []string) {
	t.Helper()
	slices.Sort(want)
	if gotNames := sortedNames(got); !slices.Equal(gotNames, want) {
		t.Fatalf("%s = %q; want %q", what, gotNames, want)
	}
}

func TestSchemaQualifiedOutsidePgCatalog(t *testing.T) {
	for text, want := range map[string]string{
		"SELECT public.lower(name) FROM t":                                       "function_schema:public.lower",
		"SELECT PUBLIC.LOWER(name) FROM t":                                       "function_schema:public.lower",
		`SELECT "Public".lower(name) FROM t`:                                     "function_schema:Public.lower",
		`SELECT "PG_CATALOG".lower(name) FROM t`:                                 "function_schema:PG_CATALOG.lower",
		`SELECT "pg_catalog ".lower(name) FROM t`:                                "function_schema:pg_catalog .lower",
		"SELECT pg_temp.f()":                                                     "function_schema:pg_temp.f",
		"SELECT information_schema._pg_expandarray(ARRAY[1])":                    "function_schema:information_schema._pg_expandarray",
		"SELECT s.f() FROM t ORDER BY 1":                                         "function_schema:s.f",
		"SELECT * FROM t ORDER BY s.f(a)":                                        "function_schema:s.f",
		"SELECT sum(a) OVER (PARTITION BY s.f(b)) FROM t":                        "function_schema:s.f",
		"SELECT count(*) FILTER (WHERE s.f(a)) FROM t":                           "function_schema:s.f",
		"SELECT percentile_cont(0.5) WITHIN GROUP (ORDER BY s.f(a)) FROM t":      "function_schema:s.f",
		"SELECT * FROM s.f()":                                                    "function_schema:s.f",
		"SELECT * FROM t, LATERAL s.f(t.a) x":                                    "function_schema:s.f",
		"WITH c AS (SELECT s.f()) SELECT * FROM c":                               "function_schema:s.f",
		"SELECT (SELECT s.f())":                                                  "function_schema:s.f",
		"EXPLAIN SELECT s.f()":                                                   "function_schema:s.f",
		"SELECT a::text FROM t WHERE CAST(s.f() AS int) > 0":                     "function_schema:s.f",
		"SELECT coalesce(a, s.f()), greatest(1, s.f()), nullif(1, s.f()) FROM t": "function_schema:s.f",
		"SELECT db.s.f()":                                                        "name_parts:db.s.f",
		"SELECT pg_catalog.pg_catalog.lower('a')":                                "name_parts:pg_catalog.pg_catalog.lower",
	} {
		assertFunctionRulesReason(t, text, want)
	}
	tree := assertFunctionRulesAccepted(t, "SELECT pg_catalog.lower(name), PG_CATALOG.upper(name), lower(name) FROM t")
	assertNames(t, "Funcs", tree.Funcs, []string{"pg_catalog.lower", "pg_catalog.upper", "lower"})
}

func TestSchemaQualifiedOperatorOutsidePgCatalog(t *testing.T) {
	for text, want := range map[string]string{
		"SELECT 1 WHERE 1 OPERATOR(public.=) 1":                                "operator_schema:public.=",
		"SELECT 1 OPERATOR(s.+) 2":                                             "operator_schema:s.+",
		`SELECT 1 OPERATOR("PG_CATALOG".+) 2`:                                  "operator_schema:PG_CATALOG.+",
		"SELECT OPERATOR(public.-) 1":                                          "operator_schema:public.-",
		"SELECT 1 WHERE 1 OPERATOR(public.=) ANY (ARRAY[1])":                   "operator_schema:public.=",
		"SELECT 1 WHERE 1 OPERATOR(public.=) ANY (SELECT 1)":                   "operator_schema:public.=",
		"SELECT 1 WHERE 1 OPERATOR(public.<) ALL (SELECT 1)":                   "operator_schema:public.<",
		"SELECT a FROM t ORDER BY a USING OPERATOR(public.<)":                  "operator_schema:public.<",
		"SELECT a FROM t WHERE EXISTS (SELECT 1 WHERE a OPERATOR(public.=) 1)": "operator_schema:public.=",
		"SELECT 1 OPERATOR(a.b.=) 1":                                           "name_parts:a.b.=",
	} {
		assertFunctionRulesReason(t, text, want)
	}
	tree := assertFunctionRulesAccepted(t, "SELECT 1 OPERATOR(pg_catalog.+) 2, a FROM t ORDER BY a USING OPERATOR(pg_catalog.<)")
	assertNames(t, "Operators", tree.Operators, []string{"pg_catalog.+", "pg_catalog.<"})
}

func TestPrefixRule(t *testing.T) {
	for text, want := range map[string]string{
		"SELECT pg_sleep(10)":                                                  "function_prefix:pg_sleep",
		"SELECT PG_SLEEP(10)":                                                  "function_prefix:pg_sleep",
		`SELECT "pg_sleep"(10)`:                                                "function_prefix:pg_sleep",
		`SELECT "PG_SLEEP"(10)`:                                                "function_prefix:PG_SLEEP",
		`SELECT U&"pg_\0073leep"(10)`:                                          "function_prefix:pg_sleep",
		"SELECT pg_catalog.pg_sleep(10)":                                       "function_prefix:pg_catalog.pg_sleep",
		"SELECT pg_advisory_lock(1)":                                           "function_prefix:pg_advisory_lock",
		"SELECT pg_advisory_xact_lock(1)":                                      "function_prefix:pg_advisory_xact_lock",
		"SELECT pg_try_advisory_lock(1)":                                       "function_prefix:pg_try_advisory_lock",
		"SELECT pg_read_file('postgresql.conf')":                               "function_prefix:pg_read_file",
		"SELECT pg_read_binary_file('x')":                                      "function_prefix:pg_read_binary_file",
		"SELECT * FROM pg_ls_dir('.')":                                         "function_prefix:pg_ls_dir",
		"SELECT pg_terminate_backend(1)":                                       "function_prefix:pg_terminate_backend",
		"SELECT pg_cancel_backend(1)":                                          "function_prefix:pg_cancel_backend",
		"SELECT pg_reload_conf()":                                              "function_prefix:pg_reload_conf",
		"SELECT pg_notify('c', 'x')":                                           "function_prefix:pg_notify",
		"SELECT pg_stat_reset()":                                               "function_prefix:pg_stat_reset",
		"SELECT pg_switch_wal()":                                               "function_prefix:pg_switch_wal",
		"SELECT * FROM pg_logical_slot_get_changes('s', NULL, NULL)":           "function_prefix:pg_logical_slot_get_changes",
		"SELECT pg_create_restore_point('x')":                                  "function_prefix:pg_create_restore_point",
		"SELECT pg_get_serial_sequence('t', 'id')":                             "function_prefix:pg_get_serial_sequence",
		"SELECT lo_import('/etc/passwd')":                                      "function_prefix:lo_import",
		"SELECT lo_export(1, '/tmp/x')":                                        "function_prefix:lo_export",
		"SELECT lo_unlink(1)":                                                  "function_prefix:lo_unlink",
		"SELECT LO_GET(1)":                                                     "function_prefix:lo_get",
		"SELECT pg_catalog.lo_from_bytea(0, 'x')":                              "function_prefix:pg_catalog.lo_from_bytea",
		"SELECT COLLATION FOR (name) FROM t":                                   "function_prefix:pg_catalog.pg_collation_for",
		"SELECT 1 FROM t ORDER BY pg_sleep(1)":                                 "function_prefix:pg_sleep",
		"SELECT count(*) FILTER (WHERE pg_sleep(1) IS NULL) FROM t":            "function_prefix:pg_sleep",
		"SELECT sum(a) OVER (ORDER BY pg_sleep(1)) FROM t":                     "function_prefix:pg_sleep",
		"WITH c AS (SELECT pg_sleep(1)) SELECT * FROM c":                       "function_prefix:pg_sleep",
		"SELECT 1 WHERE EXISTS (SELECT pg_sleep(1))":                           "function_prefix:pg_sleep",
		"SELECT * FROM ROWS FROM (generate_series(1, 2), pg_sleep(1)) r(a, b)": "function_prefix:pg_sleep",
		"SELECT coalesce(pg_sleep(1), NULL)":                                   "function_prefix:pg_sleep",
		"SELECT CASE WHEN true THEN pg_sleep(1) END":                           "function_prefix:pg_sleep",
		"SELECT ARRAY[pg_sleep(1)]":                                            "function_prefix:pg_sleep",
		"EXPLAIN SELECT pg_sleep(1)":                                           "function_prefix:pg_sleep",
	} {
		assertFunctionRulesReason(t, text, want)
	}
}

func TestPrefixAllowlist12Names(t *testing.T) {
	allowed := []string{
		"pg_typeof",
		"pg_size_pretty",
		"pg_column_size",
		"pg_relation_size",
		"pg_table_size",
		"pg_indexes_size",
		"pg_total_relation_size",
		"pg_get_viewdef",
		"pg_get_indexdef",
		"pg_get_constraintdef",
		"pg_get_expr",
		"pg_get_userbyid",
	}
	if len(allowed) != 12 {
		t.Fatalf("allowlist has %d names; want 12", len(allowed))
	}
	for _, name := range allowed {
		tree := assertFunctionRulesAccepted(t, "SELECT "+name+"(1)")
		assertNames(t, "Funcs", tree.Funcs, []string{name})
		tree = assertFunctionRulesAccepted(t, "SELECT pg_catalog."+strings.ToUpper(name)+"(1)")
		assertNames(t, "Funcs", tree.Funcs, []string{"pg_catalog." + name})
		assertFunctionRulesReason(t, "SELECT public."+name+"(1)", "function_schema:public."+name)
	}
	for _, name := range []string{"pg_typeofx", "pg_size_pretty2", "pg_get_viewdef_x", "pg_get_functiondef", "pg_get_triggerdef", "pg_relation_filepath"} {
		assertFunctionRulesReason(t, "SELECT "+name+"(1)", "function_prefix:"+name)
	}
}

func TestDenylistFamilies(t *testing.T) {
	for text, want := range map[string]string{
		"SELECT set_config('search_path', 'x', true)":                       "function_denied:set_config",
		"SELECT pg_catalog.set_config('a.b', 'x', false)":                   "function_denied:pg_catalog.set_config",
		"SELECT nextval('s')":                                               "function_denied:nextval",
		"SELECT setval('s', 1)":                                             "function_denied:setval",
		"SELECT NEXTVAL('s')":                                               "function_denied:nextval",
		`SELECT "SetVal"('s', 1)`:                                           "function_denied:SetVal",
		"SELECT txid_current()":                                             "function_denied:txid_current",
		"SELECT txid_current_if_assigned()":                                 "function_denied:txid_current_if_assigned",
		"SELECT txid_current_snapshot()":                                    "function_denied:txid_current_snapshot",
		"SELECT pg_current_xact_id()":                                       "function_denied:pg_current_xact_id",
		"SELECT pg_current_xact_id_if_assigned()":                           "function_denied:pg_current_xact_id_if_assigned",
		"SELECT * FROM ts_stat('SELECT v FROM t')":                          "function_denied:ts_stat",
		"SELECT ts_rewrite('a'::tsquery, 'SELECT t, s FROM r')":             "function_denied:ts_rewrite",
		"SELECT query_to_xml('SELECT 1', true, false, '')":                  "function_denied:query_to_xml",
		"SELECT query_to_xmlschema('SELECT 1', true, false, '')":            "function_denied:query_to_xmlschema",
		"SELECT query_to_xml_and_xmlschema('SELECT 1', true, false, '')":    "function_denied:query_to_xml_and_xmlschema",
		"SELECT cursor_to_xml('c', 1, true, false, '')":                     "function_denied:cursor_to_xml",
		"SELECT cursor_to_xmlschema('c', true, false, '')":                  "function_denied:cursor_to_xmlschema",
		"SELECT table_to_xml('pg_authid'::regclass, true, false, '')":       "function_denied:table_to_xml",
		"SELECT table_to_xml_and_xmlschema('t'::regclass, true, false, '')": "function_denied:table_to_xml_and_xmlschema",
		"SELECT schema_to_xml('public', true, false, '')":                   "function_denied:schema_to_xml",
		"SELECT schema_to_xmlschema('public', true, false, '')":             "function_denied:schema_to_xmlschema",
		"SELECT database_to_xml(true, false, '')":                           "function_denied:database_to_xml",
		"SELECT database_to_xml_and_xmlschema(true, false, '')":             "function_denied:database_to_xml_and_xmlschema",
		"SELECT brin_summarize_new_values('i'::regclass)":                   "function_denied:brin_summarize_new_values",
		"SELECT brin_summarize_range('i'::regclass, 0)":                     "function_denied:brin_summarize_range",
		"SELECT brin_desummarize_range('i'::regclass, 0)":                   "function_denied:brin_desummarize_range",
		"SELECT gin_clean_pending_list('i'::regclass)":                      "function_denied:gin_clean_pending_list",
		"SELECT loread(0, 1)":                                               "function_denied:loread",
		"SELECT lowrite(0, 'x')":                                            "function_denied:lowrite",
		"SELECT dblink('host=x', 'SELECT 1')":                               "function_denied:dblink",
		"SELECT dblink_exec('host=x', 'DROP TABLE t')":                      "function_denied:dblink_exec",
		"SELECT * FROM dblink_connect('c', 'host=x')":                       "function_denied:dblink_connect",
		"SELECT binary_upgrade_set_next_pg_type_oid(1)":                     "function_denied:binary_upgrade_set_next_pg_type_oid",
		"SELECT 1 FROM t ORDER BY nextval('s')":                             "function_denied:nextval",
		"SELECT max(a) FILTER (WHERE setval('s', 1) > 0) FROM t":            "function_denied:setval",
	} {
		assertFunctionRulesReason(t, text, want)
	}
	for _, text := range []string{
		"SELECT txid_visible_in_snapshot(1, NULL)",
		"SELECT ts_rank(v, q) FROM t",
		"SELECT currval_x(1)",
		"SELECT table_name FROM information_schema.tables",
	} {
		assertFunctionRulesAccepted(t, text)
	}
}

func TestCurrentSettingRejected(t *testing.T) {
	for text, want := range map[string]string{
		"SELECT current_setting('primary_conninfo')":               "function_denied:current_setting",
		"SELECT current_setting('primary_conninfo', true)":         "function_denied:current_setting",
		"SELECT pg_catalog.current_setting('ssl_key_file')":        "function_denied:pg_catalog.current_setting",
		"SELECT CURRENT_SETTING('search_path')":                    "function_denied:current_setting",
		`SELECT "current_setting"('search_path')`:                  "function_denied:current_setting",
		`SELECT "Current_Setting"('search_path')`:                  "function_denied:Current_Setting",
		"SELECT 1 WHERE current_setting('is_superuser') = 'on'":    "function_denied:current_setting",
		"SELECT * FROM t WHERE a IN (SELECT current_setting('x'))": "function_denied:current_setting",
		"SELECT public.current_setting('x')":                       "function_schema:public.current_setting",
	} {
		assertFunctionRulesReason(t, text, want)
	}
}

func TestFunctionNamesCollected(t *testing.T) {
	tree := assertFunctionRulesAccepted(t, `WITH c AS (SELECT upper(a) AS u FROM t)
SELECT lower(u), count(*) FILTER (WHERE length(u) > 1), sum(abs(1)) OVER (ORDER BY floor(1))
FROM c, LATERAL generate_series(1, 2) g
WHERE substring(u FROM 1 FOR 1) = 'a'
ORDER BY round(1)`)
	assertNames(t, "Funcs", tree.Funcs, []string{
		"upper", "lower", "count", "length", "sum", "abs", "floor",
		"generate_series", "pg_catalog.substring", "round",
	})
}

func TestOperatorNamesCollected(t *testing.T) {
	tree := assertFunctionRulesAccepted(t, `SELECT -a, a + 1, CASE b WHEN 1 THEN 1 END, NULLIF(a, 0)
FROM t
WHERE a BETWEEN 1 AND 2
AND a NOT BETWEEN 3 AND 4
AND a BETWEEN SYMMETRIC 5 AND 6
AND a NOT BETWEEN SYMMETRIC 7 AND 8
AND a IN (1, 2) AND a NOT IN (3)
AND a IS DISTINCT FROM b AND a IS NOT DISTINCT FROM b
AND c LIKE 'x' AND c NOT ILIKE 'y' AND c SIMILAR TO 'z'
AND a = ANY (ARRAY[1]) AND a < ALL (SELECT 1)
AND a IN (SELECT 1)
ORDER BY a USING >`)
	assertNames(t, "Operators", tree.Operators, []string{
		"-", "+", "=", "=",
		">=", "<=",
		"<", ">",
		">=", "<=",
		"<", ">",
		"=", "<>",
		"=", "=",
		"~~", "!~~*", "~",
		"=", "<",
		"=",
		">",
	})
	assertNames(t, "Funcs", tree.Funcs, []string{"pg_catalog.similar_to_escape"})
}

func TestFunctionRulesAcceptReadCorpus(t *testing.T) {
	for _, text := range readCorpus {
		assertFunctionRulesAccepted(t, text)
	}
}

func TestFixedToolTextsPassFunctionRules(t *testing.T) {
	for _, text := range []string{listTablesAllText, listTablesSchemaText, describeDefaultText, describeSchemaText} {
		tree := assertFunctionRulesAccepted(t, text)
		if len(tree.Funcs) != 0 {
			t.Fatalf("fixed text %q Funcs = %v; want none", text, tree.Funcs)
		}
		for _, op := range tree.Operators {
			if op.Schema != "" || (op.Name != "=" && op.Name != "<>") {
				t.Fatalf("fixed text %q operator %v; want = or <>", text, op)
			}
		}
	}
}
