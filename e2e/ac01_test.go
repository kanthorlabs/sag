//go:build e2e

package e2e

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kanthorlabs/sag/e2e/harness"
)

func init() {
	harness.Register(harness.AC{
		ID:    "AC01",
		Phase: harness.PhaseParallel,
		Run:   runAC01,
	})
}

func runAC01(t *testing.T, env harness.Env) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	fixtures := filepath.Join(env.RepoRoot(), "internal", "config", "testdata")
	cases := []struct {
		fixture string
		valid   bool
	}{
		{"ac01-valid", true},
		{"ac01-inline-dsn", false},
		{"ac01-unknown-key", false},
		{"ac01-max-rows", false},
		{"ac01-recheck-5s", false},
	}
	for _, c := range cases {
		t.Run(c.fixture, func(t *testing.T) {
			args := []string{"run", "--rm", "--network", "none"}
			args = append(args, harness.SecurityFlags...)
			args = append(args, "-v", fixtures+":/fixtures:ro", env.Image(), "config", "check", "--config", "/fixtures/"+c.fixture+".yaml")
			res, err := env.Podman(ctx, args...)
			if err != nil {
				t.Fatalf("podman run: %v", err)
			}
			if c.valid && res.ExitCode != 0 {
				t.Fatalf("config check exit = %d; want 0; stderr %q", res.ExitCode, res.Stderr)
			}
			if !c.valid && res.ExitCode != 1 {
				t.Fatalf("config check exit = %d; want 1; stderr %q", res.ExitCode, res.Stderr)
			}
			if strings.Contains(res.Stdout+res.Stderr, "postgres://") {
				t.Fatal("config check output holds an inline DSN")
			}
		})
	}
}
