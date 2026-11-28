package daemon

import (
	"context"
	"log/slog"
	"maps"
	"strings"

	"github.com/kanthorlabs/sag/internal/config"
	"github.com/kanthorlabs/sag/internal/core"
	"github.com/kanthorlabs/sag/internal/inventory"
	"github.com/kanthorlabs/sag/internal/scrub"
	"github.com/kanthorlabs/sag/internal/secret"
	"github.com/kanthorlabs/sag/internal/sql"
)

type AWSLoader interface {
	Load(ctx context.Context, name string) (secret.Credential, error)
}

func AWSRefs(r config.Resource) []config.SecretRef {
	var refs []config.SecretRef
	for _, ref := range []config.SecretRef{r.Connection, r.TLSCA} {
		if strings.HasPrefix(string(ref), secret.BackendAWSSM+":") {
			refs = append(refs, ref)
		}
	}
	return refs
}

func LoadAWSRefs(ctx context.Context, loader AWSLoader, r config.Resource, creds map[string]secret.Credential) (map[string]secret.Credential, error) {
	out := maps.Clone(creds)
	if out == nil {
		out = map[string]secret.Credential{}
	}
	for _, ref := range AWSRefs(r) {
		cred, err := loader.Load(ctx, strings.TrimPrefix(string(ref), secret.BackendAWSSM+":"))
		if err != nil {
			return nil, err
		}
		out[string(ref)] = cred
	}
	return out, nil
}

func awsOpenFunc(log *slog.Logger, loader AWSLoader, scrubber scrub.Scrubber, inv *inventory.Inventory, opened *resourceSet, name string, r config.Resource, creds map[string]secret.Credential, adapter core.Adapter, approvals core.ApprovalVerifier, wrap func(sql.Dialect) sql.Dialect) func(context.Context) (core.Resource, error) {
	return func(ctx context.Context) (core.Resource, error) {
		loaded, err := LoadAWSRefs(ctx, loader, r, creds)
		if err != nil {
			log.Error("aws-sm load failed", "resource", name)
			return nil, err
		}
		cred := loaded[string(r.Connection)]
		scrubber.Add(cred)
		res, err := openResource(ctx, adapter, ResourceConfig(name, r, loaded, approvals), cred, wrap)
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
