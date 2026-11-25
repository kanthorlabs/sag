package testenv

import (
	"context"
	"errors"
	"time"
)

const (
	MajorEnv     = "SAG_TEST_PG_MAJOR"
	KeepEnv      = "SAG_E2E_KEEP"
	DefaultMajor = 17
)

type Run struct {
	ID      string
	Network string
	Start   time.Time
	CA      *CA
}

func MajorFromEnv() (int, error) {
	return 0, errors.ErrUnsupported
}

func NewRun(ctx context.Context) (*Run, error) {
	return nil, errors.ErrUnsupported
}

func (r *Run) StartPostgres(ctx context.Context, major int) (*Postgres, error) {
	return nil, errors.ErrUnsupported
}

func (r *Run) CreateSecret(ctx context.Context, name string, data []byte) error {
	return errors.ErrUnsupported
}

func (r *Run) Close(ctx context.Context) error {
	return errors.ErrUnsupported
}
