package approval

import (
	"context"

	"github.com/kanthorlabs/sag/internal/core"
	"github.com/kanthorlabs/sag/internal/secret"
)

type Inspector interface {
	Inspect(ctx context.Context, cfg core.ResourceConfig, cred secret.Credential) (Target, Objects, error)
}
