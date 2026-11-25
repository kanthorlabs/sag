package sql

import (
	"context"

	"github.com/kanthorlabs/sag/internal/core"
)

type Pool interface {
	Acquire(ctx context.Context) (Conn, error)
	Close()
}

type Conn interface {
	CatalogReader
	Exec(ctx context.Context, text string) error
	ExecParams(ctx context.Context, text string, args ...any) error
	Release()
	Close() error
}

type Rows interface {
	Columns() []core.Column
	Next() bool
	Values() ([]any, error)
	Err() error
	Close()
}

type Class string

const (
	ReadOnlyViolation Class = "read_only_violation"
	Timeout           Class = "timeout"
	Connect           Class = "connect"
	Other             Class = "other"
)

type EngineError struct {
	SQLState string
	Class    Class
}

func (e *EngineError) Error() string {
	return "engine error: class " + string(e.Class) + ", SQLSTATE " + e.SQLState
}
