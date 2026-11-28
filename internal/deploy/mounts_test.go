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

func TestSecretMountsSkipAWS(t *testing.T) {
	cfg := &config.Config{
		Resources: map[string]config.Resource{
			"main": {
				Connection: config.SecretRef("aws-sm:sag/main/conn"),
				TLSCA:      config.SecretRef("aws-sm:sag/main/ca"),
			},
		},
	}
	mounts := SecretMounts(cfg)
	for _, m := range mounts {
		if m == "aws-sm:sag/main/conn" || m == "aws-sm:sag/main/ca" {
			t.Errorf("SecretMounts must not include aws-sm ref %q: %v", m, mounts)
		}
	}
}

func TestSecretMountsAWSCredentials(t *testing.T) {
	cfg := &config.Config{
		Storage: config.Storage{
			AWS: config.StorageAWS{
				Region:      "us-east-1",
				Credentials: config.SecretRef("sag-aws-credentials"),
			},
		},
	}
	mounts := SecretMounts(cfg)
	if !slices.Contains(mounts, "sag-aws-credentials") {
		t.Errorf("SecretMounts missing storage.aws.credentials: %v", mounts)
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
