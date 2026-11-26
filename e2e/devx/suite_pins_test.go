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

func TestDevxPins(t *testing.T) {
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

	dxh.RunGroup(t, ctx, r, tcfg, "pins", tagImage, func(t *testing.T) {
		t.Run("DXAC06_InstallApproval", func(t *testing.T) { DXAC06_InstallApproval(t, ctx) })
		t.Run("DXAC07_ReleaseChecksum", func(t *testing.T) { DXAC07_ReleaseChecksum(t, ctx) })
		t.Run("DXAC08_VersionRule", func(t *testing.T) { DXAC08_VersionRule(t, ctx) })
	})
}
