//go:build devxe2e

package devx_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/kanthorlabs/sag/e2e/devx/dxh"
)

func init() {
	installTests["DXAC05_PodmanAptInstall"] = DXAC05_PodmanAptInstall
}

func DXAC05_PodmanAptInstall(t *testing.T, ctx context.Context) {
	t.Helper()

	r := dxh.NewRunner()

	storeOut, err := r.Output(ctx, "podman", "info", "--format", "{{.Store.GraphRoot}}")
	if err != nil {
		t.Fatalf("DXAC05: podman info: %v", err)
	}
	graphRoot := strings.TrimSpace(string(storeOut))
	if graphRoot == "" {
		t.Fatal("DXAC05: empty graphRoot from podman info")
	}

	podmanOut, err := r.Output(ctx, "podman", "--version")
	if err != nil {
		t.Fatalf("DXAC05: podman --version before remove: %v", err)
	}
	versionBefore := parsePodmanVersion(t, podmanOut)

	if _, err := r.Output(ctx, "sudo", "-n", "apt-get", "remove", "-y", "podman"); err != nil {
		t.Fatalf("DXAC05: apt-get remove podman: %v", err)
	}
	t.Cleanup(func() {
		if _, rerr := r.Output(context.Background(), "sudo", "-n", "apt-get", "install", "-y", "podman"); rerr != nil {
			t.Logf("DXAC05 cleanup: apt-get install podman: %v", rerr)
		}
	})

	if _, statErr := os.Stat(graphRoot); statErr != nil {
		t.Fatalf("DXAC05: rootless store %q gone after apt-get remove: %v", graphRoot, statErr)
	}

	sagBin, err := dxh.TestBinary()
	if err != nil {
		t.Fatalf("DXAC05: test binary: %v", err)
	}

	groupDir := t.TempDir()
	childEnv, err := dxh.ChildEnv(os.Environ(), groupDir)
	if err != nil {
		t.Fatalf("DXAC05: child env: %v", err)
	}

	stdout, stderr, code := runBootstrapNoTerminal(t, ctx, sagBin, childEnv, "--auto-approve", "podman@5.4.2")
	if code != 0 {
		t.Fatalf("DXAC05: bootstrap exit code = %d; want 0\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "install podman") {
		t.Fatalf("DXAC05: stdout = %q; want 'install podman' line", stdout)
	}

	if _, err := r.Output(ctx, "which", "newuidmap"); err != nil {
		t.Fatalf("DXAC05: newuidmap not in PATH after install: %v", err)
	}

	podmanOut2, err := r.Output(ctx, "podman", "--version")
	if err != nil {
		t.Fatalf("DXAC05: podman --version after install: %v", err)
	}
	versionAfter := parsePodmanVersion(t, podmanOut2)
	if versionAfter != versionBefore {
		t.Fatalf("DXAC05: podman version changed: before=%q after=%q", versionBefore, versionAfter)
	}

	if _, statErr := os.Stat(graphRoot); statErr != nil {
		t.Fatalf("DXAC05: rootless store %q missing after re-install: %v", graphRoot, statErr)
	}
}

func parsePodmanVersion(t *testing.T, out []byte) string {
	t.Helper()
	parts := strings.Fields(strings.TrimSpace(string(out)))
	if len(parts) < 3 {
		t.Fatalf("DXAC05: unexpected podman --version output: %q", out)
	}
	return parts[2]
}
