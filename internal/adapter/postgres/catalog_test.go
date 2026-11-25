package postgres

import (
	"context"
	"errors"
	"slices"
	"testing"

	pg_query "github.com/pganalyze/pg_query_go/v6"
	"google.golang.org/protobuf/reflect/protoreflect"

	"github.com/kanthorlabs/sag/internal/core"
	"github.com/kanthorlabs/sag/internal/sql"
)

type catalogCheckRows struct {
	values    [][]any
	index     int
	valuesErr error
	err       error
	closed    bool
}

func (r *catalogCheckRows) Columns() []core.Column { return nil }

func (r *catalogCheckRows) Next() bool {
	r.index++
	return r.index <= len(r.values)
}

func (r *catalogCheckRows) Values() ([]any, error) {
	if r.valuesErr != nil {
		return nil, r.valuesErr
	}
	return r.values[r.index-1], nil
}

func (r *catalogCheckRows) Err() error { return r.err }
func (r *catalogCheckRows) Abort()     { r.closed = true }
func (r *catalogCheckRows) Close()     { r.closed = true }

type catalogCall struct {
	text string
	args []any
}

type catalogCheckFake struct {
	rows     map[string]*catalogCheckRows
	queryErr error
	calls    []catalogCall
}

func (f *catalogCheckFake) Query(ctx context.Context, text string, args ...any) (sql.Rows, error) {
	f.calls = append(f.calls, catalogCall{text: text, args: args})
	if f.queryErr != nil {
		return nil, f.queryErr
	}
	r, ok := f.rows[text]
	if !ok {
		return nil, errors.New("unexpected query")
	}
	return r, nil
}

func names(list ...string) []qualifiedName {
	out := make([]qualifiedName, len(list))
	for i, n := range list {
		out[i] = qualifiedName{Name: n}
	}
	return out
}

func catalogReason(t *testing.T, err error) string {
	t.Helper()
	if err == nil {
		return ""
	}
	var rejection *Rejection
	if !errors.As(err, &rejection) {
		t.Fatalf("validateCatalog error = %v (%T); want *Rejection", err, err)
	}
	return rejection.Reason()
}

func TestCatalogEmptyListsSendNoQuery(t *testing.T) {
	f := &catalogCheckFake{}
	if err := validateCatalog(context.Background(), f, &pgTree{Relations: names("t")}); err != nil {
		t.Fatalf("validateCatalog = %v; want nil", err)
	}
	if len(f.calls) != 0 {
		t.Fatalf("queries = %+v; want none", f.calls)
	}
}

func TestCatalogNamesAreBindValues(t *testing.T) {
	f := &catalogCheckFake{rows: map[string]*catalogCheckRows{
		queryFunctionCatalog: {values: [][]any{{"lower", true, false}, {"upper", true, false}}},
		queryOperatorCatalog: {values: [][]any{{"=", true, false}}},
	}}
	tree := &pgTree{
		Funcs:     []qualifiedName{{Schema: "pg_catalog", Name: "upper"}, {Name: "lower"}, {Name: "upper"}},
		Operators: names("="),
	}
	if err := validateCatalog(context.Background(), f, tree); err != nil {
		t.Fatalf("validateCatalog = %v; want nil", err)
	}
	want := []catalogCall{
		{text: queryFunctionCatalog, args: []any{[]string{"lower", "upper"}}},
		{text: queryOperatorCatalog, args: []any{[]string{"="}}},
	}
	if len(f.calls) != len(want) {
		t.Fatalf("queries = %+v; want %+v", f.calls, want)
	}
	for i, c := range f.calls {
		if c.text != want[i].text || len(c.args) != 1 || !slices.Equal(c.args[0].([]string), want[i].args[0].([]string)) {
			t.Fatalf("query %d = %+v; want %+v", i, c, want[i])
		}
	}
	for text, r := range f.rows {
		if !r.closed {
			t.Fatalf("rows of %q not closed", text)
		}
	}
}

func TestCatalogRules(t *testing.T) {
	fn := func(rows ...[]any) map[string]*catalogCheckRows {
		return map[string]*catalogCheckRows{
			queryFunctionCatalog: {values: rows},
			queryOperatorCatalog: {values: [][]any{{"=", true, false}, {"<", true, false}, {"===", false, false}}},
		}
	}
	cases := []struct {
		name  string
		rows  map[string]*catalogCheckRows
		funcs []string
		ops   []string
		want  string
	}{
		{"trusted stable", fn([]any{"now", true, false}), []string{"now"}, nil, ""},
		{"volatile allowed names", fn([]any{"random", true, true}, []any{"clock_timestamp", true, true}, []any{"gen_random_uuid", true, true}, []any{"timeofday", true, true}), []string{"random", "clock_timestamp", "gen_random_uuid", "timeofday"}, nil, ""},
		{"volatile", fn([]any{"setseed", true, true}), []string{"setseed"}, nil, "function_volatile:setseed"},
		{"one volatile overload", fn([]any{"f", true, false}, []any{"f", true, true}), []string{"f"}, nil, "function_volatile:f"},
		{"match outside pg_catalog", fn([]any{"lower", true, false}, []any{"lower", false, false}), []string{"lower"}, nil, "function_catalog:lower"},
		{"only outside pg_catalog", fn([]any{"f", false, false}), []string{"f"}, nil, "function_catalog:f"},
		{"no match", fn(), []string{"missing"}, nil, "function_catalog:missing"},
		{"first failure in tree order", fn([]any{"setseed", true, true}, []any{"g", false, false}), []string{"g", "setseed"}, nil, "function_catalog:g"},
		{"function before operator", fn([]any{"setseed", true, true}), []string{"setseed"}, []string{"==="}, "function_volatile:setseed"},
		{"operator trusted", fn(), nil, []string{"=", "<"}, ""},
		{"operator outside pg_catalog", fn(), nil, []string{"<", "==="}, "operator_catalog:==="},
		{"operator no match", fn(), nil, []string{"@@@"}, "operator_catalog:@@@"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tree := &pgTree{Funcs: names(c.funcs...), Operators: names(c.ops...)}
			got := catalogReason(t, validateCatalog(context.Background(), &catalogCheckFake{rows: c.rows}, tree))
			if got != c.want {
				t.Fatalf("reason = %q; want %q", got, c.want)
			}
		})
	}
	t.Run("qualified with pg_catalog and shadowed", func(t *testing.T) {
		tree := &pgTree{Funcs: []qualifiedName{{Schema: "pg_catalog", Name: "lower"}}}
		got := catalogReason(t, validateCatalog(context.Background(), &catalogCheckFake{rows: fn([]any{"lower", true, false}, []any{"lower", false, false})}, tree))
		if got != "function_catalog:lower" {
			t.Fatalf("reason = %q; want %q", got, "function_catalog:lower")
		}
	})
}

func TestCatalogReadErrorFailsClosed(t *testing.T) {
	engineErr := &sql.EngineError{SQLState: "25P02", Class: sql.Other}
	cases := []struct {
		name string
		fake *catalogCheckFake
	}{
		{"query error", &catalogCheckFake{queryErr: engineErr}},
		{"values error", &catalogCheckFake{rows: map[string]*catalogCheckRows{queryFunctionCatalog: {values: [][]any{{"now", true, false}}, valuesErr: errors.New("decode")}}}},
		{"rows error", &catalogCheckFake{rows: map[string]*catalogCheckRows{queryFunctionCatalog: {values: [][]any{{"now", true, false}}, err: engineErr}}}},
		{"short row", &catalogCheckFake{rows: map[string]*catalogCheckRows{queryFunctionCatalog: {values: [][]any{{"now", true}}}}}},
		{"name not text", &catalogCheckFake{rows: map[string]*catalogCheckRows{queryFunctionCatalog: {values: [][]any{{nil, true, false}}}}}},
		{"trusted not bool", &catalogCheckFake{rows: map[string]*catalogCheckRows{queryFunctionCatalog: {values: [][]any{{"now", nil, false}}}}}},
		{"volatile not bool", &catalogCheckFake{rows: map[string]*catalogCheckRows{queryFunctionCatalog: {values: [][]any{{"now", true, "v"}}}}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := validateCatalog(context.Background(), c.fake, &pgTree{Funcs: names("now")})
			if err == nil {
				t.Fatal("validateCatalog = nil; want an error")
			}
			var rejection *Rejection
			if errors.As(err, &rejection) {
				t.Fatalf("validateCatalog = %v; want the read error, not a rejection", err)
			}
			if r := c.fake.rows[queryFunctionCatalog]; r != nil && !r.closed {
				t.Fatal("rows not closed after the read error")
			}
			if got := queryRejected(err); got == nil {
				t.Fatal("queryRejected = nil; want an error")
			}
		})
	}
}

func TestCatalogQueriesImmuneToSearchPath(t *testing.T) {
	for _, text := range []string{queryFunctionCatalog, queryOperatorCatalog} {
		result, err := pg_query.Parse(text)
		if err != nil {
			t.Fatalf("parse %q: %v", text, err)
		}
		qualified := func(list []*pg_query.Node) bool {
			return len(list) == 2 && list[0].GetString_().GetSval() == "pg_catalog"
		}
		err = eachMessage(result.ProtoReflect(), func(m protoreflect.Message) error {
			switch n := m.Interface().(type) {
			case *pg_query.RangeVar:
				if n.Schemaname != "pg_catalog" {
					return errors.New("relation " + n.Relname + " is not qualified")
				}
			case *pg_query.FuncCall:
				if !qualified(n.Funcname) {
					return errors.New("function is not qualified")
				}
			case *pg_query.A_Expr:
				if !qualified(n.Name) {
					return errors.New("operator is not qualified")
				}
			case *pg_query.TypeName:
				if !qualified(n.Names) {
					return errors.New("type is not qualified")
				}
			case *pg_query.ParamRef, *pg_query.A_Const, *pg_query.ColumnRef, *pg_query.ResTarget, *pg_query.JoinExpr,
				*pg_query.RangeFunction, *pg_query.Alias, *pg_query.BoolExpr, *pg_query.TypeCast, *pg_query.SelectStmt,
				*pg_query.RawStmt, *pg_query.ParseResult, *pg_query.Node, *pg_query.List, *pg_query.String,
				*pg_query.Integer, *pg_query.Boolean, *pg_query.A_Star:
			default:
				return errors.New("unexpected node " + string(m.Descriptor().FullName()))
			}
			return nil
		})
		if err != nil {
			t.Fatalf("%q: %v", text, err)
		}
	}
}
