package daemon

import (
	"context"
	"log/slog"

	"github.com/kanthorlabs/sag/internal/core"
	"github.com/kanthorlabs/sag/internal/inventory"
	"github.com/kanthorlabs/sag/internal/scrub"
	"github.com/kanthorlabs/sag/internal/secret"
	"github.com/kanthorlabs/sag/internal/sql"
)

type awsLoader interface {
	Load(ctx context.Context, name string) (secret.Credential, error)
}

func awsOpenFunc(log *slog.Logger, loader awsLoader, scrubber scrub.Scrubber, inv *inventory.Inventory, opened *resourceSet, name, awsName string, adapter core.Adapter, rc core.ResourceConfig, wrap func(sql.Dialect) sql.Dialect) func(context.Context) (core.Resource, error) {
	return func(ctx context.Context) (core.Resource, error) {
		cred, err := loader.Load(ctx, awsName)
		if err != nil {
			log.Error("aws-sm load failed", "resource", name)
			return nil, err
		}
		scrubber.Add(cred)
		res, err := openResource(ctx, adapter, rc, cred, wrap)
		if err != nil {
			log.Error("resource open failed", "resource", name, "error", err.Error())
			return nil, err
		}
		if err := inv.SetResource(name, res); err != nil {
			_ = res.Close()
			return nil, err
		}
		opened.add(res)
		return res, nil
	}
}
