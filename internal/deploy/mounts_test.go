package deploy

import (
	"slices"
	"testing"

	"github.com/kanthorlabs/sag/internal/config"
	"github.com/kanthorlabs/sag/internal/secret"
)

func TestSecretMountsMasterKey(t *testing.T) {
	cfg := &config.Config{}
	mounts := SecretMounts(cfg)
	found := slices.Contains(mounts, secret.MasterKeyName)
	if !found {
		t.Fatalf("SecretMounts missing master key %q: %v", secret.MasterKeyName, mounts)
	}
}

func TestSecretMountsSecretRefs(t *testing.T) {
	cfg := &config.Config{
		Server: config.Server{
			TLSCert: config.SecretRef("sag-tls-cert"),
			TLSKey:  config.SecretRef("sag-tls-key"),
		},
		Resources: map[string]config.Resource{
			"main": {
				Connection: config.SecretRef("sag-main-conn"),
			},
			"replica": {
				Connection: config.SecretRef("sag-replica-conn"),
				TLSCA:      config.SecretRef("sag-replica-ca"),
			},
		},
	}
	mounts := SecretMounts(cfg)
	for _, want := range []string{
		secret.MasterKeyName,
		"sag-tls-cert",
		"sag-tls-key",
		"sag-main-conn",
		"sag-replica-conn",
		"sag-replica-ca",
	} {
		if !slices.Contains(mounts, want) {
			t.Errorf("SecretMounts missing %q: %v", want, mounts)
		}
	}
}

func TestSecretMountsNoDuplicates(t *testing.T) {
	cfg := &config.Config{
		Server: config.Server{
			TLSCert: config.SecretRef("sag-tls-cert"),
			TLSKey:  config.SecretRef("sag-tls-cert"),
		},
	}
	mounts := SecretMounts(cfg)
	seen := map[string]bool{}
	for _, m := range mounts {
		if seen[m] {
			t.Fatalf("duplicate mount %q: %v", m, mounts)
		}
		seen[m] = true
	}
}
