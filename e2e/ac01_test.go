//go:build e2e

package e2e

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kanthorlabs/sag/e2e/harness"
	"github.com/kanthorlabs/sag/internal/secret"
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
	t.Run("ac01-valid", func(t *testing.T) {
		if _, err := os.Stat(secret.Dir); !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("host %s must not exist for the valid case: stat error %v", secret.Dir, err)
		}
		bin := filepath.Join(t.TempDir(), "sag")
		build := exec.CommandContext(ctx, "go", "build", "-o", bin, "./cmd/sag")
		build.Dir = env.RepoRoot()
		if out, err := build.CombinedOutput(); err != nil {
			t.Fatalf("go build: %v: %s", err, out)
		}
		check := exec.CommandContext(ctx, bin, "config", "check", "--config", filepath.Join(fixtures, "ac01-valid.yaml"))
		out, err := check.CombinedOutput()
		if err != nil {
			t.Fatalf("config check: %v; output %q", err, out)
		}
		if strings.Contains(string(out), "postgres://") {
			t.Fatal("config check output holds an inline DSN")
		}
	})

	for _, fixture := range []string{"ac01-inline-dsn", "ac01-unknown-key", "ac01-max-rows", "ac01-recheck-5s"} {
		t.Run(fixture, func(t *testing.T) {
			args := []string{"run", "--rm", "--network", "none"}
			args = append(args, harness.SecurityFlags...)
			args = append(args, "-v", fixtures+":/fixtures:ro", env.Image(), "config", "check", "--config", "/fixtures/"+fixture+".yaml")
			res, err := env.Podman(ctx, args...)
			if err != nil {
				t.Fatalf("podman run: %v", err)
			}
			if res.ExitCode != 1 {
				t.Fatalf("config check exit = %d; want 1; stderr %q", res.ExitCode, res.Stderr)
			}
			if strings.Contains(res.Stdout+res.Stderr, "postgres://") {
				t.Fatal("config check output holds an inline DSN")
			}
		})
	}
}
