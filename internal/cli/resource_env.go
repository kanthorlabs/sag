package cli

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"

	"github.com/kanthorlabs/sag/internal/approval"
	"github.com/kanthorlabs/sag/internal/config"
	"github.com/kanthorlabs/sag/internal/core"
	"github.com/kanthorlabs/sag/internal/daemon"
	"github.com/kanthorlabs/sag/internal/keys"
	"github.com/kanthorlabs/sag/internal/secret"
	"github.com/kanthorlabs/sag/internal/secret/awssm"
)

type resourceTarget struct {
	Adapter   core.Adapter
	Config    core.ResourceConfig
	Cred      secret.Credential
	Approvals *approval.Store
}

var (
	loadResource  = resourceFromPaths
	loadApprovals = approvalsFromPaths
)

func approvalsFromPaths() (*approval.Store, error) {
	creds, err := secret.Load(paths.Secrets, paths.SecretUID, []string{secret.MasterKeyName})
	if err != nil {
		return nil, err
	}
	master, err := secret.DecodeMasterKey(creds[secret.MasterKeyName])
	if err != nil {
		return nil, err
	}
	key, err := keys.ApprovalKey(master)
	if err != nil {
		return nil, err
	}
	return approval.NewStore(filepath.Join(paths.State, approval.DirName), key), nil
}

func resourceFromPaths(name string) (resourceTarget, error) {
	cfg, err := config.Load(paths.Config)
	if err != nil {
		return resourceTarget{}, err
	}
	r, ok := cfg.Resources[name]
	if !ok {
		return resourceTarget{}, fmt.Errorf("resource %q is not in the config", name)
	}
	adapter, ok := core.Lookup(r.Kind)
	if !ok {
		return resourceTarget{}, fmt.Errorf("resource %q has unknown kind %q", name, r.Kind)
	}

	names := []string{secret.MasterKeyName}
	for _, ref := range []config.SecretRef{r.Connection, r.TLSCA} {
		if ref != "" && !slices.Contains(daemon.AWSRefs(r), ref) {
			names = append(names, string(ref))
		}
	}
	creds, err := secret.Load(paths.Secrets, paths.SecretUID, names)
	if err != nil {
		return resourceTarget{}, err
	}
	if len(daemon.AWSRefs(r)) > 0 {
		ctx := context.Background()
		backend, err := awssm.NewDaemon(ctx, cfg.Storage.AWS.Region, string(cfg.Storage.AWS.Credentials))
		if err != nil {
			return resourceTarget{}, fmt.Errorf("resource %q: aws-sm backend: %w", name, err)
		}
		creds, err = daemon.LoadAWSRefs(ctx, backend, r, creds)
		if err != nil {
			return resourceTarget{}, fmt.Errorf("resource %q: load from aws-sm: %w", name, err)
		}
	}

	master, err := secret.DecodeMasterKey(creds[secret.MasterKeyName])
	if err != nil {
		return resourceTarget{}, err
	}
	key, err := keys.ApprovalKey(master)
	if err != nil {
		return resourceTarget{}, err
	}
	store := approval.NewStore(filepath.Join(paths.State, approval.DirName), key)
	return resourceTarget{
		Adapter:   adapter,
		Config:    daemon.ResourceConfig(name, r, creds, store),
		Cred:      creds[string(r.Connection)],
		Approvals: store,
	}, nil
}
