package postgres

import (
	"context"
	"slices"

	"github.com/kanthorlabs/sag/internal/sql"
)

const (
	queryFunctionCatalog = `SELECT q.want, p.oid OPERATOR(pg_catalog.<) 16384::pg_catalog.oid AND n.nspname OPERATOR(pg_catalog.=) 'pg_catalog', p.provolatile OPERATOR(pg_catalog.=) 'v' FROM pg_catalog.unnest($1::pg_catalog.text[]) AS q(want) JOIN pg_catalog.pg_proc p ON p.proname OPERATOR(pg_catalog.=) q.want::pg_catalog.name JOIN pg_catalog.pg_namespace n ON n.oid OPERATOR(pg_catalog.=) p.pronamespace`
	queryOperatorCatalog = `SELECT q.want, o.oid OPERATOR(pg_catalog.<) 16384::pg_catalog.oid AND n.nspname OPERATOR(pg_catalog.=) 'pg_catalog', false FROM pg_catalog.unnest($1::pg_catalog.text[]) AS q(want) JOIN pg_catalog.pg_operator o ON o.oprname OPERATOR(pg_catalog.=) q.want::pg_catalog.name JOIN pg_catalog.pg_namespace n ON n.oid OPERATOR(pg_catalog.=) o.oprnamespace`
)

var volatileAllowed = []string{"random", "clock_timestamp", "gen_random_uuid", "timeofday"}

type catalogMatches struct {
	builtin   bool
	untrusted bool
	volatile  bool
}

func validateCatalog(ctx context.Context, tx sql.CatalogReader, t *pgTree) error {
	funcs, err := readCatalogMatches(ctx, tx, queryFunctionCatalog, slices.Concat(t.Funcs, t.FieldNames))
	if err != nil {
		return err
	}
	for _, f := range t.Funcs {
		if err := checkFunctionMatches(f.Name, funcs[f.Name]); err != nil {
			return err
		}
	}
	for _, f := range t.FieldNames {
		if m := funcs[f.Name]; m.builtin || m.untrusted {
			if err := checkFunctionMatches(f.Name, m); err != nil {
				return err
			}
		}
	}
	ops, err := readCatalogMatches(ctx, tx, queryOperatorCatalog, t.Operators)
	if err != nil {
		return err
	}
	for _, o := range t.Operators {
		m := ops[o.Name]
		if m.untrusted || !m.builtin {
			return &Rejection{Rule: ruleOperatorCatalog, Name: o.Name}
		}
	}
	return nil
}

func checkFunctionMatches(name string, m catalogMatches) error {
	if m.untrusted || !m.builtin {
		return &Rejection{Rule: ruleFunctionCatalog, Name: name}
	}
	if m.volatile && !slices.Contains(volatileAllowed, name) {
		return &Rejection{Rule: ruleFunctionVolatile, Name: name}
	}
	return nil
}

func readCatalogMatches(ctx context.Context, tx sql.CatalogReader, query string, list []qualifiedName) (map[string]catalogMatches, error) {
	matches := map[string]catalogMatches{}
	if len(list) == 0 {
		return matches, nil
	}
	bare := make([]string, 0, len(list))
	for _, n := range list {
		bare = append(bare, n.Name)
	}
	slices.Sort(bare)
	rows, err := tx.Query(ctx, query, slices.Compact(bare))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		values, err := rows.Values()
		if err != nil {
			return nil, err
		}
		if len(values) != 3 {
			return nil, errCatalogValue
		}
		name, okName := values[0].(string)
		builtin, okBuiltin := values[1].(bool)
		volatile, okVolatile := values[2].(bool)
		if !okName || !okBuiltin || !okVolatile {
			return nil, errCatalogValue
		}
		m := matches[name]
		m.builtin = m.builtin || builtin
		m.untrusted = m.untrusted || !builtin
		m.volatile = m.volatile || (builtin && volatile)
		matches[name] = m
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return matches, nil
}
