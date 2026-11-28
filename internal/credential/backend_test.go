package credential

import (
	"testing"

	"github.com/kanthorlabs/sag/internal/config"
	"github.com/kanthorlabs/sag/internal/secret"
)

func TestBackendNeedsStorageAWS(t *testing.T) {
	cfg := &config.Config{}
	_, err := BackendFor(cfg, secret.BackendAWSSM)
	if err == nil {
		t.Fatal("BackendFor aws-sm with empty region must return error")
	}
}

func TestAWSSecretNames(t *testing.T) {
	cases := []struct {
		backend  string
		name     string
		wantConn string
		wantCA   string
	}{
		{
			backend:  secret.BackendAWSSM,
			name:     "mydb",
			wantConn: "sag/mydb/conn",
			wantCA:   "sag/mydb/ca",
		},
		{
			backend:  secret.BackendPodman,
			name:     "mydb",
			wantConn: "sag-res-mydb.conn",
			wantCA:   "sag-res-mydb.ca",
		},
	}
	for _, c := range cases {
		if got := secretConnName(c.backend, c.name); got != c.wantConn {
			t.Errorf("secretConnName(%q,%q) = %q, want %q", c.backend, c.name, got, c.wantConn)
		}
		if got := secretCAName(c.backend, c.name); got != c.wantCA {
			t.Errorf("secretCAName(%q,%q) = %q, want %q", c.backend, c.name, got, c.wantCA)
		}
	}
}
