//go:build e2e

package e2e

import (
	"context"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kanthorlabs/sag/e2e/harness"
)

const (
	ac07Module      = "github.com/kanthorlabs/sag"
	ac07PassThrough = ac07Module + "/e2e/conformance/passthrough"
	ac07TestOnly    = ac07Module + "/e2e/"
	ac07CLISymbol   = ac07Module + "/internal/cli.Run"
	ac07BinaryPath  = "/usr/local/bin/sag"
)

func init() {
	harness.Register(harness.AC{
		ID:    "AC07",
		Phase: harness.PhaseParallel,
		Run:   runAC07,
	})
}

func runAC07(t *testing.T, env harness.Env) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	conformanceDeps := strings.Fields(ac07Go(ctx, t, env, "list", "-deps", "./e2e/conformance/cmd/sag-conformance"))
	if !slices.Contains(conformanceDeps, ac07PassThrough) {
		t.Fatalf("go list -deps of sag-conformance does not list %s; the check cannot detect the package", ac07PassThrough)
	}
	for _, dep := range strings.Fields(ac07Go(ctx, t, env, "list", "-deps", "./cmd/sag")) {
		if strings.HasPrefix(dep, ac07TestOnly) {
			t.Errorf("go list -deps ./cmd/sag lists test-only package %s", dep)
		}
	}

	conformanceSymbols := ac07Symbols(ctx, t, env, harness.ConformanceImage, "conformance")
	if !strings.Contains(conformanceSymbols, ac07PassThrough+".") {
		t.Fatalf("symbol table of %s has no %s symbol; the check cannot detect the package", harness.ConformanceImage, ac07PassThrough)
	}
	product := ac07Symbols(ctx, t, env, env.Image(), "product")
	if !strings.Contains(product, ac07CLISymbol) {
		t.Fatalf("symbol table of %s has no %s; the binary has no readable symbol table", env.Image(), ac07CLISymbol)
	}
	for _, line := range strings.Split(product, "\n") {
		if strings.Contains(line, ac07TestOnly) || strings.Contains(line, "passthrough") {
			t.Errorf("symbol table of %s holds %q", env.Image(), line)
		}
	}
}

func ac07Go(ctx context.Context, t *testing.T, env harness.Env, args ...string) string {
	t.Helper()
	cmd := exec.CommandContext(ctx, "go", args...)
	cmd.Dir = env.RepoRoot()
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}

func ac07Symbols(ctx context.Context, t *testing.T, env harness.Env, image, label string) string {
	t.Helper()
	name := env.ContainerName("ac07-" + label)
	if res, err := env.Podman(ctx, "create", "--name", name, image); err != nil || res.ExitCode != 0 {
		t.Fatalf("podman create %s: %v %s", image, err, res.Stderr)
	}
	defer env.Podman(context.WithoutCancel(ctx), "rm", "--force", "--volumes", name)
	bin := filepath.Join(t.TempDir(), "sag")
	if res, err := env.Podman(ctx, "cp", name+":"+ac07BinaryPath, bin); err != nil || res.ExitCode != 0 {
		t.Fatalf("podman cp %s:%s: %v %s", name, ac07BinaryPath, err, res.Stderr)
	}
	return ac07Go(ctx, t, env, "tool", "nm", bin)
}
