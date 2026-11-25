package postgres

import (
	"errors"
	"fmt"
	"slices"
	"strings"
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

func TestRelationsRecorded(t *testing.T) {
	tree := mustParseTree(t, `WITH c AS (SELECT * FROM s1.a)
SELECT * FROM b JOIN "S2"."B" ON true, LATERAL (SELECT * FROM c) l
WHERE x IN (SELECT y FROM pg_catalog.pg_class)
UNION ALL SELECT * FROM pg_roles AS pg_authid, pg_user`)
	if err := checkRelations(tree); err != nil {
		t.Fatalf("checkRelations() = %v; want nil", err)
	}
	want := []qualifiedName{
		{Schema: "", Name: "b"},
		{Schema: "", Name: "c"},
		{Schema: "", Name: "pg_roles"},
		{Schema: "", Name: "pg_user"},
		{Schema: "S2", Name: "B"},
		{Schema: "pg_catalog", Name: "pg_class"},
		{Schema: "s1", Name: "a"},
	}
	got := slices.Clone(tree.Relations)
	slices.SortFunc(got, func(a, b qualifiedName) int {
		if c := strings.Compare(a.Schema, b.Schema); c != 0 {
			return c
		}
		return strings.Compare(a.Name, b.Name)
	})
	if !slices.Equal(got, want) {
		t.Fatalf("Relations = %v; want %v", got, want)
	}
}

func TestRelationsNoneForPlainSelect(t *testing.T) {
	tree := mustParseTree(t, "SELECT 1, pg_typeof(1), 'pg_authid'::regclass")
	if err := checkRelations(tree); err != nil || len(tree.Relations) != 0 {
		t.Fatalf("checkRelations() = %v with %v; want nil and no relations", err, tree.Relations)
	}
}

func TestFixedToolTextsPassShapeAndRelations(t *testing.T) {
	for text, want := range map[string]qualifiedName{
		"SELECT table_schema AS schema, table_name AS name, table_type AS type FROM information_schema.tables WHERE table_schema NOT IN ('pg_catalog', 'information_schema') ORDER BY table_schema, table_name": {Schema: "information_schema", Name: "tables"},
		"SELECT table_schema AS schema, table_name AS name, table_type AS type FROM information_schema.tables WHERE table_schema = 'public' ORDER BY table_schema, table_name":                                  {Schema: "information_schema", Name: "tables"},
		"SELECT column_name AS name, data_type AS type, is_nullable AS nullable FROM information_schema.columns WHERE table_schema = 'public' AND table_name = 'pg_authid' ORDER BY ordinal_position":           {Schema: "information_schema", Name: "columns"},
	} {
		tree := mustParseTree(t, text)
		if err := checkStatementShape(tree); err != nil {
			t.Fatalf("checkStatementShape(%q) = %v; want nil", text, err)
		}
		if err := checkRelations(tree); err != nil || !slices.Equal(tree.Relations, []qualifiedName{want}) {
			t.Fatalf("checkRelations(%q) = %v with %v; want nil and %v", text, err, tree.Relations, want)
		}
	}
}
