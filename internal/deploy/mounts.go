package deploy

import (
	"maps"
	"slices"

	"github.com/kanthorlabs/sag/internal/config"
	"github.com/kanthorlabs/sag/internal/secret"
)

func SecretMounts(cfg *config.Config) []string {
	names := map[string]struct{}{}
	if cfg.Server.TLSCert != "" {
		names[string(cfg.Server.TLSCert)] = struct{}{}
	}
	if cfg.Server.TLSKey != "" {
		names[string(cfg.Server.TLSKey)] = struct{}{}
	}
	for _, r := range cfg.Resources {
		if r.Connection != "" {
			names[string(r.Connection)] = struct{}{}
		}
		if r.TLSCA != "" {
			names[string(r.TLSCA)] = struct{}{}
		}
	}
	delete(names, secret.MasterKeyName)
	rest := slices.Sorted(maps.Keys(names))
	return append([]string{secret.MasterKeyName}, rest...)
}
