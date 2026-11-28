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

var installTests = map[string]func(*testing.T, context.Context){}

func TestDevxInstall(t *testing.T) {
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
		QuadletExists: func() bool {
			return pathExists(filepath.Join(home, ".config", "containers", "systemd", "sag.container"))
		},
	}
	devDir, err := dxh.SharedDevDir()
	if err != nil {
		t.Fatal(err)
	}
	markerFile := filepath.Join(devDir, "local", "devx-run-active")

	cleanup := dxh.SuiteEntry(t, r, pcfg, tcfg, markerFile)
	defer cleanup()

	installOrder := []string{
		"DXAC05_PodmanAptInstall",
	}
	dxh.RunGroup(t, ctx, r, tcfg, "install", nil, func(t *testing.T) {
		for _, name := range installOrder {
			name := name
			fn := installTests[name]
			t.Run(name, func(t *testing.T) {
				if fn == nil {
					t.Fatal("not implemented")
				}
				fn(t, ctx)
			})
		}
	})
}
