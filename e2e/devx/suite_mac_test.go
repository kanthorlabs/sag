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

var macTests = map[string]func(*testing.T, context.Context){}

func TestDevxMac(t *testing.T) {
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

	tagImage := func() error { return dxh.GroupSetupImage(ctx, r) }

	groups := []struct {
		name  string
		tests []string
	}{
		{"macboot", []string{"DXAC34_MacBootstrap"}},
		{"maccred", []string{"DXAC35_MacCredential"}},
		{"macpkg", []string{"DXAC38_MacPkgChecksum"}},
		{"mactailscale", []string{"DXAC52_MacTailscale"}},
	}
	for _, g := range groups {
		dxh.RunGroup(t, ctx, r, tcfg, g.name, tagImage, func(t *testing.T) {
			for _, name := range g.tests {
				fn := macTests[name]
				t.Run(name, func(t *testing.T) {
					if fn == nil {
						t.Fatal("not implemented")
					}
					fn(t, ctx)
				})
			}
		})
	}
}
