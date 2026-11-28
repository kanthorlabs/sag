//go:build devxe2e

package devx_test

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strings"
	"testing"

	"github.com/kanthorlabs/sag/e2e/devx/dxh"
)

func DXAC08_VersionRule(t *testing.T, ctx context.Context) {
	t.Helper()

	if runtime.GOOS == "linux" {
		dxac08AptVersionRule(t, ctx)
		return
	}

	sagBin, err := dxh.TestBinary()
	if err != nil {
		t.Fatalf("DXAC08: test binary: %v", err)
	}

	r := dxh.NewRunner()
	versionOut, err := r.Output(ctx, "podman", "--version")
	if err != nil {
		t.Fatalf("DXAC08: podman --version: %v", err)
	}
	versionParts := strings.Fields(strings.TrimSpace(string(versionOut)))
	if len(versionParts) < 3 {
		t.Fatalf("DXAC08: podman --version unexpected output: %q", versionOut)
	}
	installedVersion := versionParts[2]

	t.Setenv("SAG_TEST_ABSENT", "")

	olderGroupDir := t.TempDir()
	olderBinDir := t.TempDir()

	t.Setenv("SAG_TEST_PINS", "podman 999.0.0 darwin/arm64 release https://example.invalid/podman.pkg 0000000000000000000000000000000000000000000000000000000000000000\n")
	olderChildEnv, err := dxh.ChildEnv(os.Environ(), olderGroupDir)
	if err != nil {
		t.Fatalf("DXAC08 older: child env: %v", err)
	}
	olderChildEnv = append(olderChildEnv, "PATH="+dxh.TestPATH(olderBinDir))

	stdout, stderr, code := runBootstrapNoTerminal(t, ctx, sagBin, olderChildEnv)
	if code != 1 {
		t.Fatalf("DXAC08 older: exit code = %d; want 1\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}
	expectedFail := fmt.Sprintf("fail podman installed %s, pin 999.0.0", installedVersion)
	if !strings.Contains(stdout, expectedFail) {
		t.Fatalf("DXAC08 older: stdout = %q; want %q line", stdout, expectedFail)
	}
	if _, statErr := os.Stat(dxh.CfgDir(os.Getenv("HOME"))); statErr == nil {
		t.Fatal("DXAC08 older: cfg dir exists after failed bootstrap")
	}

	newerGroupDir := t.TempDir()
	newerBinDir := t.TempDir()

	t.Setenv("SAG_TEST_PINS", "podman 0.0.1 darwin/arm64 release https://example.invalid/podman.pkg 0000000000000000000000000000000000000000000000000000000000000000\n")
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

	versionOut2, err := r.Output(ctx, "podman", "--version")
	if err != nil {
		t.Fatalf("DXAC08: podman --version after tests: %v", err)
	}
	versionParts2 := strings.Fields(strings.TrimSpace(string(versionOut2)))
	if len(versionParts2) < 3 {
		t.Fatalf("DXAC08: podman --version after tests unexpected output: %q", versionOut2)
	}
	if versionParts2[2] != installedVersion {
		t.Fatalf("DXAC08: podman version changed: before=%q after=%q", installedVersion, versionParts2[2])
	}
}

func dxac08AptVersionRule(t *testing.T, ctx context.Context) {
	t.Helper()

	sagBin, err := dxh.TestBinary()
	if err != nil {
		t.Fatalf("DXAC08 apt: test binary: %v", err)
	}

	t.Setenv("SAG_TEST_ABSENT", "")

	olderGroupDir := t.TempDir()
	t.Setenv("SAG_TEST_PINS", "podman 999.0.0 linux/* apt podman -\n")
	olderChildEnv, err := dxh.ChildEnv(os.Environ(), olderGroupDir)
	if err != nil {
		t.Fatalf("DXAC08 apt older: child env: %v", err)
	}

	stdout, stderr, code := runBootstrapNoTerminal(t, ctx, sagBin, olderChildEnv)
	if code != 1 {
		t.Fatalf("DXAC08 apt older: exit code = %d; want 1\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "fail podman") {
		t.Fatalf("DXAC08 apt older: stdout = %q; want 'fail podman' line", stdout)
	}
	if _, statErr := os.Stat(dxh.CfgDir(os.Getenv("HOME"))); statErr == nil {
		t.Fatal("DXAC08 apt older: cfg dir exists after failed bootstrap")
	}

	newerGroupDir := t.TempDir()
	t.Setenv("SAG_TEST_PINS", "podman 1.0.0 linux/* apt podman -\n")
	newerChildEnv, err := dxh.ChildEnv(os.Environ(), newerGroupDir)
	if err != nil {
		t.Fatalf("DXAC08 apt newer: child env: %v", err)
	}

	stdout, stderr, code = runBootstrapNoTerminal(t, ctx, sagBin, newerChildEnv)
	if code != 0 {
		t.Fatalf("DXAC08 apt newer: exit code = %d; want 0\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "warn podman") {
		t.Fatalf("DXAC08 apt newer: stdout = %q; want 'warn podman' line", stdout)
	}
	warnCount := strings.Count(stdout, "warn podman")
	if warnCount != 1 {
		t.Fatalf("DXAC08 apt newer: found %d 'warn podman' lines; want exactly 1", warnCount)
	}
}
