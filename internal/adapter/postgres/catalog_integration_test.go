//go:build integration

package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kanthorlabs/sag/internal/sql"
	"github.com/kanthorlabs/sag/internal/testenv"
)

const catalogHostileSearchPath = "t34_evil, pg_catalog, public"

var catalogHostileFixture = []string{
	"CREATE SCHEMA t34_evil",
	"CREATE FUNCTION t34_evil.never(name, name) RETURNS boolean LANGUAGE sql IMMUTABLE AS 'SELECT false'",
	"CREATE OPERATOR t34_evil.= (LEFTARG = name, RIGHTARG = name, FUNCTION = t34_evil.never)",
	"CREATE FUNCTION t34_evil.never_oid(oid, oid) RETURNS boolean LANGUAGE sql IMMUTABLE AS 'SELECT false'",
	"CREATE OPERATOR t34_evil.= (LEFTARG = oid, RIGHTARG = oid, FUNCTION = t34_evil.never_oid)",
	"CREATE FUNCTION t34_evil.lower(text) RETURNS text LANGUAGE sql STABLE AS 'SELECT $1'",
	"CREATE FUNCTION t34_evil.t34_only() RETURNS int LANGUAGE sql STABLE AS 'SELECT 1'",
	"CREATE SCHEMA t34_hidden",
	"CREATE FUNCTION t34_hidden.length(text) RETURNS int LANGUAGE sql IMMUTABLE AS 'SELECT 0'",
	"CREATE FUNCTION t34_hidden.plus(int, int) RETURNS int LANGUAGE sql IMMUTABLE AS 'SELECT $1 + $2'",
	"CREATE OPERATOR t34_hidden.@#@ (LEFTARG = int, RIGHTARG = int, FUNCTION = t34_hidden.plus)",
	"CREATE FUNCTION pg_catalog.t34_injected() RETURNS int LANGUAGE sql STABLE AS 'SELECT 1'",
	"ALTER ROLE CURRENT_USER SET search_path = " + catalogHostileSearchPath,
}

type catalogCase struct {
	name  string
	funcs []qualifiedName
	ops   []qualifiedName
	want  string
}

func catalogRequest(t *testing.T, pg *testenv.Postgres) (context.Context, sql.Conn) {
	t.Helper()
	cfg := testResourceConfig(pg, 1)
	p := openPool(t, pg, cfg)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)
	c := acquire(t, ctx, p)
	t.Cleanup(func() { _ = c.Close() })
	beginRequest(t, ctx, c, testLimits, cfg)
	return ctx, c
}

func runCatalogCases(t *testing.T, ctx context.Context, c sql.Conn, cases []catalogCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := catalogReason(t, validateCatalog(ctx, c, &pgTree{Funcs: tc.funcs, Operators: tc.ops, Validated: true}))
			if got != tc.want {
				t.Fatalf("reason = %q; want %q", got, tc.want)
			}
		})
	}
}

func TestVolatilityGate(t *testing.T) {
	pg := startPostgres(t)
	ctx, c := catalogRequest(t, pg)
	runCatalogCases(t, ctx, c, []catalogCase{
		{"random passes", names("random"), nil, ""},
		{"named volatile list passes", names("random", "clock_timestamp", "gen_random_uuid", "timeofday"), nil, ""},
		{"stable and immutable pass", names("now", "abs", "lower", "count", "row_number", "to_char"), nil, ""},
		{"setseed rejects", names("setseed"), nil, "function_volatile:setseed"},
		{"random then setseed", names("random", "setseed"), nil, "function_volatile:setseed"},
		{"nextval rejects", names("nextval"), nil, "function_volatile:nextval"},
		{"advisory lock rejects", names("pg_advisory_lock"), nil, "function_volatile:pg_advisory_lock"},
		{"ts_stat rejects", names("ts_stat"), nil, "function_volatile:ts_stat"},
		{"pg_sleep rejects", names("pg_sleep"), nil, "function_volatile:pg_sleep"},
		{"qualified setseed rejects", []qualifiedName{{Schema: "pg_catalog", Name: "setseed"}}, nil, "function_volatile:setseed"},
	})
}

func TestFunctionOutsidePgCatalog(t *testing.T) {
	pg := startPostgres(t)
	fixture(t, pg, catalogHostileFixture...)
	ctx, c := catalogRequest(t, pg)
	if got := show(t, ctx, c, "search_path"); got != catalogHostileSearchPath {
		t.Fatalf("search_path = %q; want the hostile role setting %q", got, catalogHostileSearchPath)
	}
	runCatalogCases(t, ctx, c, []catalogCase{
		{"shadow in search_path rejects", names("lower"), nil, "function_catalog:lower"},
		{"pg_catalog qualified shadow rejects", []qualifiedName{{Schema: "pg_catalog", Name: "lower"}}, nil, "function_catalog:lower"},
		{"name only outside pg_catalog rejects", names("t34_only"), nil, "function_catalog:t34_only"},
		{"match outside search_path rejects", names("length"), nil, "function_catalog:length"},
		{"user function in pg_catalog rejects", names("t34_injected"), nil, "function_catalog:t34_injected"},
		{"unknown name rejects", names("t34_missing"), nil, "function_catalog:t34_missing"},
		{"built-in passes under hostile search_path", names("upper", "abs", "now"), nil, ""},
		{"volatile reason under hostile operators", names("setseed"), nil, "function_volatile:setseed"},
	})
	if got := show(t, ctx, c, "search_path"); got != catalogHostileSearchPath {
		t.Fatalf("search_path after validateCatalog = %q; want %q unchanged", got, catalogHostileSearchPath)
	}
}

func TestOperatorOutsidePgCatalog(t *testing.T) {
	pg := startPostgres(t)
	fixture(t, pg, catalogHostileFixture...)
	ctx, c := catalogRequest(t, pg)
	runCatalogCases(t, ctx, c, []catalogCase{
		{"fake = in search_path rejects", nil, names("="), "operator_catalog:="},
		{"operator outside search_path rejects", nil, names("@#@"), "operator_catalog:@#@"},
		{"unknown operator rejects", nil, names("@@@@"), "operator_catalog:@@@@"},
		{"built-in operators pass", nil, names("<", "<>", "||", "~~", ">="), ""},
		{"function checked before operator", names("lower"), names("<"), "function_catalog:lower"},
	})
	if got := show(t, ctx, c, "search_path"); got != catalogHostileSearchPath {
		t.Fatalf("search_path after validateCatalog = %q; want %q unchanged", got, catalogHostileSearchPath)
	}
}

func TestBrinGinRejected(t *testing.T) {
	pg := startPostgres(t)
	ctx, c := catalogRequest(t, pg)
	runCatalogCases(t, ctx, c, []catalogCase{
		{"brin_summarize_new_values", names("brin_summarize_new_values"), nil, "function_volatile:brin_summarize_new_values"},
		{"brin_summarize_range", names("brin_summarize_range"), nil, "function_volatile:brin_summarize_range"},
		{"brin_desummarize_range", names("brin_desummarize_range"), nil, "function_volatile:brin_desummarize_range"},
		{"gin_clean_pending_list", names("gin_clean_pending_list"), nil, "function_volatile:gin_clean_pending_list"},
	})
}

func TestCatalogReadErrorFailsClosedOnServer(t *testing.T) {
	pg := startPostgres(t)
	ctx, c := catalogRequest(t, pg)
	if _, err := queryValues(ctx, c, "SELECT 1 / 0"); err == nil {
		t.Fatal("SELECT 1 / 0 succeeded; want an error that aborts the transaction")
	}
	for _, tree := range []*pgTree{
		{Funcs: names("now"), Validated: true},
		{Operators: names("="), Validated: true},
	} {
		err := validateCatalog(ctx, c, tree)
		var engineErr *sql.EngineError
		if !errors.As(err, &engineErr) || engineErr.SQLState != "25P02" {
			t.Fatalf("validateCatalog in an aborted transaction = %v; want EngineError 25P02", err)
		}
	}
}
