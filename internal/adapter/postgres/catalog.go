package postgres

import (
	"context"

	"github.com/kanthorlabs/sag/internal/sql"
)

func validateCatalog(ctx context.Context, tx sql.CatalogReader, t *pgTree) error {
	return notImplemented("validateCatalog")
}
