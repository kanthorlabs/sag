package postgres

import (
	"errors"
	"fmt"
	"testing"
)

var credentialCatalogNames = []string{
	"pg_authid",
	"pg_shadow",
	"pg_user_mapping",
	"pg_user_mappings",
	"pg_subscription",
	"pg_settings",
	"pg_file_settings",
	"pg_statistic",
	"pg_stats",
	"pg_statistic_ext_data",
	"pg_stats_ext",
	"pg_stats_ext_exprs",
	"user_mapping_options",
	"_pg_user_mappings",
}

func assertRelationsReason(t *testing.T, text, want string) {
	t.Helper()
	err := checkRelations(mustParseTree(t, text))
	var rejection *Rejection
	if !errors.As(err, &rejection) {
		t.Fatalf("checkRelations(%q) = %v; want a Rejection %q", text, err, want)
	}
	if got := rejection.Reason(); got != want {
		t.Fatalf("checkRelations(%q) reason = %q; want %q", text, got, want)
	}
}

func TestCredentialCatalogsAnyDepth(t *testing.T) {
	shapes := []string{
		"SELECT * FROM %s",
		"TABLE %s",
		"SELECT 1 FROM t JOIN %s x ON true",
		"SELECT 1 FROM t LEFT JOIN (t2 CROSS JOIN %s) ON true",
		"SELECT * FROM (SELECT * FROM %s) s",
		"SELECT 1 WHERE EXISTS (SELECT 1 FROM %s)",
		"SELECT 1 WHERE 'x' IN (SELECT 1 FROM %s)",
		"SELECT (SELECT count(*) FROM %s)",
		"WITH c AS (SELECT * FROM %s) SELECT * FROM c",
		"WITH a AS (WITH b AS (SELECT * FROM %s) SELECT * FROM b) SELECT * FROM a",
		"SELECT 1 UNION ALL SELECT 1 FROM %s",
		"SELECT * FROM t, LATERAL (SELECT * FROM %s) l",
		"SELECT * FROM t ORDER BY (SELECT 1 FROM %s LIMIT 1)",
		"SELECT count(*) FILTER (WHERE EXISTS (SELECT 1 FROM %s)) FROM t",
		"EXPLAIN SELECT * FROM %s",
		"EXPLAIN (FORMAT JSON) WITH c AS (SELECT 1 FROM %s) SELECT * FROM c",
		"SELECT * FROM %s TABLESAMPLE SYSTEM (10)",
		"SELECT * FROM ONLY %s",
		"WITH %[1]s AS (SELECT 1) SELECT * FROM %[1]s",
	}
	for _, name := range credentialCatalogNames {
		for _, shape := range shapes {
			assertRelationsReason(t, fmt.Sprintf(shape, name), "relation:"+name)
		}
	}
	for text, want := range map[string]string{
		"SELECT * FROM PG_AUTHID":         "relation:pg_authid",
		`SELECT * FROM "PG_AUTHID"`:       "relation:PG_AUTHID",
		`SELECT * FROM "Pg_Shadow"`:       "relation:Pg_Shadow",
		`SELECT * FROM "pg_settings"`:     "relation:pg_settings",
		"SELECT * FROM Pg_File_Settings":  "relation:pg_file_settings",
		`SELECT * FROM U&"pg_\0061uthid"`: "relation:pg_authid",
	} {
		assertRelationsReason(t, text, want)
	}
}

func TestCredentialCatalogsAnySchema(t *testing.T) {
	for _, name := range credentialCatalogNames {
		for _, schema := range []string{"pg_catalog", "public", "information_schema", "pg_temp", "s"} {
			assertRelationsReason(t, "SELECT * FROM "+schema+"."+name, "relation:"+schema+"."+name)
		}
		assertRelationsReason(t, `SELECT * FROM "PG_CATALOG"."`+name+`"`, "relation:PG_CATALOG."+name)
		assertRelationsReason(t, "SELECT * FROM PG_CATALOG."+name, "relation:pg_catalog."+name)
		assertRelationsReason(t, "SELECT * FROM postgres.pg_catalog."+name, "name_parts:postgres.pg_catalog."+name)
	}
	assertRelationsReason(t, "SELECT * FROM db.public.t", "name_parts:db.public.t")
	assertRelationsReason(t, "SELECT * FROM t JOIN db.s.u ON true", "name_parts:db.s.u")
}

func TestRelationsAccepted(t *testing.T) {
	for _, text := range []string{
		`WITH c AS (SELECT * FROM s1.a)
SELECT * FROM b JOIN "S2"."B" ON true, LATERAL (SELECT * FROM c) l
WHERE x IN (SELECT y FROM pg_catalog.pg_class)
UNION ALL SELECT * FROM pg_roles AS pg_authid, pg_user`,
		"SELECT 1, pg_typeof(1), 'pg_authid'::regclass",
		"SELECT * FROM pg_statistic_ext, pg_user_mappings_x, information_schema.user_mappings",
	} {
		if err := checkRelations(mustParseTree(t, text)); err != nil {
			t.Fatalf("checkRelations(%q) = %v; want nil", text, err)
		}
	}
}

func TestFixedToolTextsPassShapeAndRelations(t *testing.T) {
	for _, text := range []string{
		"SELECT table_schema AS schema, table_name AS name, table_type AS type FROM information_schema.tables WHERE table_schema NOT IN ('pg_catalog', 'information_schema') ORDER BY table_schema, table_name",
		"SELECT table_schema AS schema, table_name AS name, table_type AS type FROM information_schema.tables WHERE table_schema = 'public' ORDER BY table_schema, table_name",
		"SELECT column_name AS name, data_type AS type, is_nullable AS nullable FROM information_schema.columns WHERE table_schema = 'public' AND table_name = 'pg_authid' ORDER BY ordinal_position",
	} {
		tree := mustParseTree(t, text)
		if err := checkStatementShape(tree); err != nil {
			t.Fatalf("checkStatementShape(%q) = %v; want nil", text, err)
		}
		if err := checkRelations(tree); err != nil {
			t.Fatalf("checkRelations(%q) = %v; want nil", text, err)
		}
	}
}
