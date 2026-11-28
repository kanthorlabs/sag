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

	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		t.Fatalf("suite entry: XDG_CONFIG_HOME=%q is set; unset it before running e2e-devx", xdg)
	}
	if err := dxh.CheckRunLock(filepath.Dir(filepath.Dir(markerFile))); err != nil {
		t.Fatalf("suite entry: %v", err)
	}
	if _, err := os.Stat(markerFile); err == nil {
		pcfg.Marker = true
	}
	if err := dxh.CheckLinuxPreconditionsWithRecovery(ctx, r, pcfg, func(ctx context.Context) error {
		return dxh.Teardown(ctx, r, tcfg)
	}); err != nil {
		t.Fatalf("suite precondition: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(markerFile), 0o700); err != nil {
		t.Fatalf("suite entry mkdir: %v", err)
	}
	if err := os.WriteFile(markerFile, nil, 0o600); err != nil {
		t.Fatalf("suite entry write marker: %v", err)
	}
	defer func() { _ = os.Remove(markerFile) }()

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
