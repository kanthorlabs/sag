package main

import (
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const (
	guardModule      = "github.com/kanthorlabs/sag"
	guardPassThrough = guardModule + "/e2e/conformance/passthrough"
	guardTestOnly    = guardModule + "/e2e/"
	guardProductMain = "./cmd/sag"
	guardConformance = "./e2e/conformance/cmd/sag-conformance"
	guardCLISymbol   = guardModule + "/internal/cli.Run"
)

func guardRepoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func guardGo(t *testing.T, args ...string) string {
	t.Helper()
	cmd := exec.Command("go", args...)
	cmd.Dir = guardRepoRoot(t)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}

func guardDeps(t *testing.T, pkg string) []string {
	t.Helper()
	return strings.Fields(guardGo(t, "list", "-deps", pkg))
}

func guardSymbols(t *testing.T, pkg string) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "bin")
	guardGo(t, "build", "-trimpath", "-o", bin, pkg)
	return guardGo(t, "tool", "nm", bin)
}

func TestAC07_NoPassThroughDep(t *testing.T) {
	conformance := guardDeps(t, guardConformance)
	if !slices.Contains(conformance, guardPassThrough) {
		t.Fatalf("go list -deps %s does not list %s; the guard cannot detect the package", guardConformance, guardPassThrough)
	}
	for _, dep := range guardDeps(t, guardProductMain) {
		if strings.HasPrefix(dep, guardTestOnly) {
			t.Errorf("go list -deps %s lists test-only package %s", guardProductMain, dep)
		}
	}
}

func TestAC07_NoPassThroughSymbol(t *testing.T) {
	conformance := guardSymbols(t, guardConformance)
	if !strings.Contains(conformance, guardPassThrough+".") {
		t.Fatalf("symbol table of %s has no %s symbol; the guard cannot detect the package", guardConformance, guardPassThrough)
	}
	product := guardSymbols(t, guardProductMain)
	if !strings.Contains(product, guardCLISymbol) {
		t.Fatalf("symbol table of %s has no %s; the binary has no readable symbol table", guardProductMain, guardCLISymbol)
	}
	for _, line := range strings.Split(product, "\n") {
		if strings.Contains(line, guardTestOnly) || strings.Contains(line, "passthrough") {
			t.Errorf("symbol table of %s holds %q", guardProductMain, line)
		}
	}
}
