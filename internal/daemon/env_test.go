package daemon

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRemovesPGVariables(t *testing.T) {
	t.Setenv("PGHOST", "db.example.net")
	t.Setenv("PGPASSWORD", "pg-env-password")
	t.Setenv("PGSSLMODE", "disable")
	t.Setenv("SAG_KEEP", "kept")
	_ = Run(context.Background(), Options{
		ConfigPath: filepath.Join(t.TempDir(), "missing.yaml"),
		SecretsDir: t.TempDir(),
		SecretUID:  os.Getuid(),
		StateDir:   t.TempDir(),
		Version:    "test",
		Log:        &strings.Builder{},
	})
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "PG") {
			t.Fatalf("environment still holds %q", strings.SplitN(kv, "=", 2)[0])
		}
	}
	if os.Getenv("SAG_KEEP") != "kept" {
		t.Fatal("Run removed a variable without the PG prefix")
	}
}
