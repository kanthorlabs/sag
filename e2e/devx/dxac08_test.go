//go:build devxe2e

package devx_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/kanthorlabs/sag/e2e/devx/dxh"
)

func DXAC08_VersionRule(t *testing.T, ctx context.Context) {
	t.Helper()

	sagBin, err := dxh.TestBinary()
	if err != nil {
		t.Fatalf("DXAC08: test binary: %v", err)
	}

	t.Setenv("SAG_TEST_ABSENT", "")

	olderGroupDir := t.TempDir()
	olderBinDir := t.TempDir()

	t.Setenv("SAG_TEST_PINS", "podman 999.0.0 darwin/arm64 release - -\n")
	olderChildEnv, err := dxh.ChildEnv(os.Environ(), olderGroupDir)
	if err != nil {
		t.Fatalf("DXAC08 older: child env: %v", err)
	}
	olderChildEnv = append(olderChildEnv, "PATH="+dxh.TestPATH(olderBinDir))

	stdout, stderr, code := runBootstrapNoTerminal(t, ctx, sagBin, olderChildEnv)
	if code != 1 {
		t.Fatalf("DXAC08 older: exit code = %d; want 1\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "fail podman") {
		t.Fatalf("DXAC08 older: stdout = %q; want 'fail podman' line", stdout)
	}

	newerGroupDir := t.TempDir()
	newerBinDir := t.TempDir()

	t.Setenv("SAG_TEST_PINS", "podman 0.0.1 darwin/arm64 release - -\n")
	newerChildEnv, err := dxh.ChildEnv(os.Environ(), newerGroupDir)
	if err != nil {
		t.Fatalf("DXAC08 newer: child env: %v", err)
	}
	newerChildEnv = append(newerChildEnv, "PATH="+dxh.TestPATH(newerBinDir))

	stdout, stderr, code = runBootstrapNoTerminal(t, ctx, sagBin, newerChildEnv)
	if code != 0 {
		t.Fatalf("DXAC08 newer: exit code = %d; want 0\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "warn podman") {
		t.Fatalf("DXAC08 newer: stdout = %q; want 'warn podman' line", stdout)
	}
	warnCount := strings.Count(stdout, "warn podman")
	if warnCount != 1 {
		t.Fatalf("DXAC08 newer: found %d 'warn podman' lines; want exactly 1", warnCount)
	}
}
