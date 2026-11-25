//go:build e2e

package e2e

import (
	"context"
	"crypto/rand"
	"encoding/hex"
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
	"github.com/kanthorlabs/sag/internal/testenv"
)

func init() {
	harness.Register(harness.AC{
		ID:    "AC01",
		Phase: harness.PhaseParallel,
		Run:   runAC01,
	})
}

var ac01Secrets = []string{"sag-tls-cert", "sag-tls-key", "sag-orders", "sag-partner-ledger", secret.MasterKeyName}

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

	t.Run("ac01-valid-in-image", func(t *testing.T) {
		suffix := make([]byte, 4)
		if _, err := rand.Read(suffix); err != nil {
			t.Fatal(err)
		}
		args := []string{"run", "--rm", "--network", "none"}
		args = append(args, harness.SecurityFlags...)
		for _, name := range ac01Secrets {
			source := "sag-e2e-ac01-" + hex.EncodeToString(suffix) + "-" + name
			res, err := testenv.Podman(ctx, strings.NewReader("x"), "secret", "create", source, "-")
			if err != nil || res.ExitCode != 0 {
				t.Fatalf("podman secret create %s: %v %s", source, err, res.Stderr)
			}
			t.Cleanup(func() { testenv.Podman(context.Background(), nil, "secret", "rm", source) })
			args = append(args, "--secret", harness.SecretFlag(source, name, harness.DefaultSecretMode))
		}
		args = append(args, "-v", fixtures+":/fixtures:ro", env.Image(), "config", "check", "--config", "/fixtures/ac01-valid.yaml")
		res, err := env.Podman(ctx, args...)
		if err != nil {
			t.Fatalf("podman run: %v", err)
		}
		if res.ExitCode != 0 || !strings.Contains(res.Stdout, "config ok") {
			t.Fatalf("config check exit = %d, stdout %q, stderr %q; want 0 and config ok", res.ExitCode, res.Stdout, res.Stderr)
		}
	})

	invalid := []struct {
		fixture string
		cause   string
	}{
		{"ac01-inline-dsn", "resources.orders.connection: must be a secret reference"},
		{"ac01-unknown-key", "line 27: unknown key"},
		{"ac01-max-rows", "defaults.limits.max_rows: 1001 exceeds the hard maximum 1000"},
		{"ac01-recheck-5s", "preflight.recheck_interval: must be from 10s to 10m0s"},
	}
	for _, c := range invalid {
		t.Run(c.fixture, func(t *testing.T) {
			args := []string{"run", "--rm", "--network", "none"}
			args = append(args, harness.SecurityFlags...)
			args = append(args, "-v", fixtures+":/fixtures:ro", env.Image(), "config", "check", "--config", "/fixtures/"+c.fixture+".yaml")
			res, err := env.Podman(ctx, args...)
			if err != nil {
				t.Fatalf("podman run: %v", err)
			}
			if res.ExitCode != 1 {
				t.Fatalf("config check exit = %d; want 1; stderr %q", res.ExitCode, res.Stderr)
			}
			if !strings.Contains(res.Stderr, c.cause) {
				t.Fatalf("config check stderr = %q; want the cause %q", res.Stderr, c.cause)
			}
			if strings.Contains(res.Stderr, secret.Dir) {
				t.Fatalf("config check stderr = %q; want no secret file error", res.Stderr)
			}
			if strings.Contains(res.Stdout+res.Stderr, "postgres://") {
				t.Fatal("config check output holds an inline DSN")
			}
		})
	}
}
