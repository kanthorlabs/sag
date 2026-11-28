//go:build devxe2e

package devx_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kanthorlabs/sag/e2e/devx/dxh"
)

var userHarnessTests = map[string]func(*testing.T, context.Context){}

var userRealBinDir string

func TestDevxUser(t *testing.T) {
	if err := dxh.CheckNoClaude(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := dxh.CheckProfileEnv(); err != nil {
		t.Fatal(err)
	}

	realBinDir := t.TempDir()
	for _, prog := range []string{"claude", "pi", "node"} {
		realPath, err := findRealProgram(prog)
		if err != nil {
			t.Fatalf("find real %s: %v", prog, err)
		}
		if err := dxh.AddRealProgramLink(realBinDir, prog, realPath); err != nil {
			t.Fatalf("link real %s: %v", prog, err)
		}
	}
	userRealBinDir = realBinDir

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	r := dxh.NewRunner()
	home := os.Getenv("HOME")
	tcfg := dxh.TeardownConfig{
		Home:   home,
		UID:    fmt.Sprintf("%d", os.Getuid()),
		CfgDir: cfgDir(home),
	}
	pcfg := dxh.PreconditionConfig{
		CfgDir:    cfgDir(home),
		PortHeld:  func() bool { return isPortHeld("127.0.0.1:8443") },
		CfgExists: func() bool { return pathExists(cfgDir(home)) },
		LaunchAgentExists: func() bool {
			return pathExists(filepath.Join(home, "Library", "LaunchAgents", "net.kanthorlabs.sag.plist"))
		},
	}
	devDir, err := dxh.SharedDevDir()
	if err != nil {
		t.Fatal(err)
	}
	markerFile := filepath.Join(devDir, "local", "devx-run-active")

	cleanup := dxh.SuiteEntry(t, r, pcfg, tcfg, markerFile)
	defer cleanup()

	snapDir := filepath.Join(devDir, "local", "devx-user", time.Now().UTC().Format("20060102T150405Z"))
	snap, err := dxh.Snapshot(snapDir)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}

	if err := dxh.CheckNoSAGEntries(home); err != nil {
		t.Fatalf("pre-run SAG check: %v", err)
	}

	tagImage := func() error { return dxh.GroupSetupImage(ctx, r) }

	userHarnessOrder := []string{
		"DXAC39_DefaultPath",
		"DXAC47_RealClaudeCode",
		"DXAC48_RealPi",
	}

	dxh.RunGroup(t, ctx, r, tcfg, "harness", tagImage, func(t *testing.T) {
		for _, name := range userHarnessOrder {
			name := name
			fn := userHarnessTests[name]
			t.Run(name, func(t *testing.T) {
				if fn == nil {
					t.Fatal("not implemented")
				}
				fn(t, ctx)
			})
		}
	})

	if err := dxh.TeardownHarnessFiles(ctx, r, home); err != nil {
		t.Errorf("teardown harness files: %v", err)
	}

	if err := dxh.CompareSAGEntries(snap); err != nil {
		t.Fatalf("compare sag entries: %v", err)
	}
}

func findRealProgram(name string) (string, error) {
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		p := filepath.Join(dir, name)
		if info, err := os.Stat(p); err == nil && !info.IsDir() && info.Mode()&0o111 != 0 {
			return p, nil
		}
	}
	return "", fmt.Errorf("real program %q not found in PATH", name)
}
