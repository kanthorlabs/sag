package deploy

import (
	"maps"
	"slices"
	"strings"

	"github.com/kanthorlabs/sag/internal/config"
	"github.com/kanthorlabs/sag/internal/secret"
)

func SecretMounts(cfg *config.Config) []string {
	names := map[string]struct{}{}
	for _, ref := range []config.SecretRef{cfg.Server.TLSCert, cfg.Server.TLSKey} {
		if ref != "" && !isAWSRef(ref) {
			names[string(ref)] = struct{}{}
		}
	}
	if cfg.Storage.AWS.Credentials != "" {
		names[string(cfg.Storage.AWS.Credentials)] = struct{}{}
	}
	for _, r := range cfg.Resources {
		if r.Connection != "" && !isAWSRef(r.Connection) {
			names[string(r.Connection)] = struct{}{}
		}
		if r.TLSCA != "" && !isAWSRef(r.TLSCA) {
			names[string(r.TLSCA)] = struct{}{}
		}
	}
	delete(names, secret.MasterKeyName)
	rest := slices.Sorted(maps.Keys(names))
	return append([]string{secret.MasterKeyName}, rest...)
}

func isAWSRef(ref config.SecretRef) bool {
	return strings.HasPrefix(string(ref), "aws-sm:")
}
