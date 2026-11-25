package testenv

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

type Postgres struct {
	Major     int
	Container string
	HostPort  int
	CAPEM     []byte
}

func StartPostgres(ctx context.Context, major int) (*Postgres, error) {
	return nil, errors.ErrUnsupported
}

func (p *Postgres) HostDSN() string {
	return ""
}

func (p *Postgres) NetworkDSN() string {
	return ""
}

func (p *Postgres) Connect(ctx context.Context) (*pgx.Conn, error) {
	return nil, errors.ErrUnsupported
}

func (p *Postgres) Logs(ctx context.Context, since time.Time) (string, error) {
	return "", errors.ErrUnsupported
}

func (p *Postgres) Close(ctx context.Context) error {
	return errors.ErrUnsupported
}
