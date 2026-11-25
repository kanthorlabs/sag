package postgres

import (
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/kanthorlabs/sag/internal/core"
	"github.com/kanthorlabs/sag/internal/secret"
)

func connConfig(cfg core.ResourceConfig, cred secret.Credential) (*pgx.ConnConfig, error) {
	return nil, errors.ErrUnsupported
}
