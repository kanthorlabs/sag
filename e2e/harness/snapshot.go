package harness

import (
	"context"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/kanthorlabs/sag/internal/testenv"
)

const databasesQuery = "SELECT datname FROM pg_catalog.pg_database WHERE datallowconn AND NOT datistemplate ORDER BY datname"

type DatabaseSnapshot struct {
	Major    int
	Database string
	Rows     [][]string
}

func Databases(ctx context.Context, pg *testenv.Postgres) ([]string, error) {
	conn, err := ConnectDB(ctx, pg, defaultDatabase)
	if err != nil {
		return nil, err
	}
	defer conn.Close(context.WithoutCancel(ctx))
	rows, err := conn.Query(ctx, databasesQuery)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[string])
}

func QueryText(ctx context.Context, conn *pgx.Conn, query string) ([][]string, error) {
	rows, err := conn.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) ([]string, error) {
		values := make([]string, len(row.FieldDescriptions()))
		dest := make([]any, len(values))
		for i := range values {
			dest[i] = &values[i]
		}
		return values, row.Scan(dest...)
	})
}

func (s *Suite) takeSnapshots(ctx context.Context) error {
	s.snapshots = map[string][]DatabaseSnapshot{}
	majors := make([]int, 0, len(s.pgs))
	for m := range s.pgs {
		majors = append(majors, m)
	}
	slices.Sort(majors)
	for _, ac := range s.acs {
		if ac.Snapshot == "" {
			continue
		}
		for _, m := range majors {
			pg := s.pgs[m]
			dbs, err := Databases(ctx, pg)
			if err != nil {
				return fmt.Errorf("harness: %s snapshot: list databases of PostgreSQL %d: %w", ac.ID, m, err)
			}
			for _, db := range dbs {
				rows, err := s.snapshotDatabase(ctx, pg, db, ac.Snapshot)
				if err != nil {
					return fmt.Errorf("harness: %s snapshot of PostgreSQL %d database %s: %w", ac.ID, m, db, err)
				}
				s.snapshots[ac.ID] = append(s.snapshots[ac.ID], DatabaseSnapshot{Major: m, Database: db, Rows: rows})
			}
		}
	}
	return nil
}

func (s *Suite) snapshotDatabase(ctx context.Context, pg *testenv.Postgres, db, query string) ([][]string, error) {
	conn, err := ConnectDB(ctx, pg, db)
	if err != nil {
		return nil, err
	}
	defer conn.Close(context.WithoutCancel(ctx))
	return QueryText(ctx, conn, query)
}

func (s *Suite) Snapshots(id string) []DatabaseSnapshot {
	return slices.Clone(s.snapshots[id])
}
