package daemon

import (
	"context"
	"log/slog"
	"maps"
	"slices"
	"strings"

	"github.com/kanthorlabs/sag/internal/config"
	"github.com/kanthorlabs/sag/internal/core"
	"github.com/kanthorlabs/sag/internal/inventory"
	"github.com/kanthorlabs/sag/internal/scrub"
	"github.com/kanthorlabs/sag/internal/secret"
	"github.com/kanthorlabs/sag/internal/secret/awssm"
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

var newAWSDaemon = func(ctx context.Context, region, credentialsName string) (AWSLoader, error) {
	return awssm.NewDaemon(ctx, region, credentialsName)
}

type failedAWSLoader struct{ err error }

func (l failedAWSLoader) Load(context.Context, string) (secret.Credential, error) {
	return secret.Credential{}, l.err
}

func needsAWS(cfg *config.Config) bool {
	for _, r := range cfg.Resources {
		if len(AWSRefs(r)) > 0 {
			return true
		}
	}
	return false
}

func newScrubber(cfg *config.Config, creds map[string]secret.Credential) scrub.Scrubber {
	scrubber := scrub.New(slices.Collect(maps.Values(creds)))
	if cfg.Storage.AWS.Credentials != "" {
		if file, ok := creds[string(cfg.Storage.AWS.Credentials)]; ok {
			for _, v := range awsCredentialValues(file.Reveal()) {
				scrubber.Add(secret.NewCredential(string(cfg.Storage.AWS.Credentials), []byte(v)))
			}
		}
	}
	return scrubber
}

func awsCredentialValues(file []byte) []string {
	var out []string
	for _, line := range strings.Split(string(file), "\n") {
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		switch strings.TrimSpace(key) {
		case "aws_access_key_id", "aws_secret_access_key", "aws_session_token":
			if v := strings.TrimSpace(value); v != "" {
				out = append(out, v)
			}
		}
	}
	return out
}

func awsOpenFunc(log *slog.Logger, loader AWSLoader, scrubber scrub.Scrubber, inv *inventory.Inventory, opened *resourceSet, name string, r config.Resource, creds map[string]secret.Credential, adapter core.Adapter, approvals core.ApprovalVerifier, wrap func(sql.Dialect) sql.Dialect) func(context.Context) (core.Resource, error) {
	return func(ctx context.Context) (core.Resource, error) {
		loaded, err := LoadAWSRefs(ctx, loader, r, creds)
		if err != nil {
			log.Error("aws-sm load failed", "resource", name)
			return nil, err
		}
		for _, ref := range AWSRefs(r) {
			scrubber.Add(loaded[string(ref)])
		}
		cred := loaded[string(r.Connection)]
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
