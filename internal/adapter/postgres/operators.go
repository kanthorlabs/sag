package postgres

import (
	"context"
	"errors"

	"github.com/kanthorlabs/sag/internal/sql"
)

func operatorCollisions(ctx context.Context, r sql.CatalogReader) ([]string, error) {
	return nil, errors.ErrUnsupported
}
