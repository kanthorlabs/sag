//go:build devxe2e

package devx_test

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kanthorlabs/sag/e2e/devx/dxh"
)

var credentialTests = map[string]func(*testing.T, context.Context){}

var certTests = map[string]func(*testing.T, context.Context){}

var deleteTests = map[string]func(*testing.T, context.Context){}

var hostformsTests = map[string]func(*testing.T, context.Context){}

var agentTests = map[string]func(*testing.T, context.Context){}

var bootagentTests = map[string]func(*testing.T, context.Context){}

var imageTests = map[string]func(*testing.T, context.Context){}

func TestDevxMain(t *testing.T) {
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

	groupDir := t.TempDir()
	binDir := t.TempDir()

	dxh.RunGroup(t, ctx, r, tcfg, "harness", tagImage, func(t *testing.T) {
		t.Run("DX_Harness_Skeleton", func(t *testing.T) {
			testHarnessSkeleton(t, ctx, groupDir, binDir)
		})
	})

	dxh.RunGroup(t, ctx, r, tcfg, "base", tagImage, func(t *testing.T) {
		t.Run("DXAC01_Bootstrap", func(t *testing.T) { DXAC01_Bootstrap(t, ctx) })
		t.Run("DXAC02_ContainerAndCert", func(t *testing.T) { DXAC02_ContainerAndCert(t, ctx) })
		t.Run("DXAC03_BootstrapIdempotent", func(t *testing.T) { DXAC03_BootstrapIdempotent(t, ctx) })
	})

	dxh.RunGroup(t, ctx, r, tcfg, "noterm", tagImage, func(t *testing.T) {
		t.Run("DXAC04_BootstrapNoTerminal", func(t *testing.T) {
			testDXAC04BootstrapNoTerminal(t, ctx, groupDir, binDir)
		})
	})

	dxh.RunGroup(t, ctx, r, tcfg, "guards", tagImage, func(t *testing.T) {
		testDXAC33_ProductBinary(t, ctx, groupDir, binDir)
		testDXAC36_HostGuards(t, ctx, groupDir, binDir)
	})

	dxh.RunGroup(t, ctx, r, tcfg, "service", tagImage, func(t *testing.T) {
		t.Run("DXAC27_Autostart", func(t *testing.T) { DXAC27_Autostart(t, ctx) })
		t.Run("DXAC28_ServiceStopStart", func(t *testing.T) { DXAC28_ServiceStopStart(t, ctx) })
	})

	dxh.RunGroup(t, ctx, r, tcfg, "tailscale", tagImage, func(t *testing.T) {
		t.Run("DXAC50_TailscaleFake", func(t *testing.T) { DXAC50_TailscaleFake(t, ctx) })
	})

	dxh.RunGroup(t, ctx, r, tcfg, "port", tagImage, func(t *testing.T) {
		t.Run("DXAC53_PortHolder", func(t *testing.T) { DXAC53_PortHolder(t, ctx) })
	})

	credentialOrder := []string{
		"DXAC09_CreateCanary",
		"DXAC10_CanaryNoLeak",
		"DXAC11_CreateRejectsForm",
		"DXAC12_ConnectionCheck",
		"DXAC13_LoopbackWarn",
		"DXAC14_CreateStdin",
		"DXAC15_CreateSecretExists",
		"DXAC16_CreateRollback",
		"DXAC17_ListInspect",
		"DXAC37_CreateTLSCA",
	}
	dxh.RunGroup(t, ctx, r, tcfg, "credential", tagImage, func(t *testing.T) {
		for _, name := range credentialOrder {
			name := name
			fn := credentialTests[name]
			t.Run(name, func(t *testing.T) {
				if fn == nil {
					t.Fatal("not implemented")
				}
				fn(t, ctx)
			})
		}
	})

	certOrder := []string{"DXAC29_CertRenewal"}
	dxh.RunGroup(t, ctx, r, tcfg, "cert", tagImage, func(t *testing.T) {
		for _, name := range certOrder {
			name := name
			fn := certTests[name]
			t.Run(name, func(t *testing.T) {
				if fn == nil {
					t.Fatal("not implemented")
				}
				fn(t, ctx)
			})
		}
	})

	deleteOrder := []string{
		"DXAC18_DeleteNoTerminal",
		"DXAC19_DeleteConfirm",
		"DXAC20_Delete",
		"DXAC21_DeleteStopped",
		"DXAC22_DeleteResume",
		"DXAC23_DeleteForeignSecret",
	}
	dxh.RunGroup(t, ctx, r, tcfg, "delete", tagImage, func(t *testing.T) {
		for _, name := range deleteOrder {
			name := name
			fn := deleteTests[name]
			t.Run(name, func(t *testing.T) {
				if fn == nil {
					t.Fatal("not implemented")
				}
				fn(t, ctx)
			})
		}
	})

	hostformsOrder := []string{
		"DXAC24_HostJWTGenerate",
		"DXAC25_Logs",
	}
	dxh.RunGroup(t, ctx, r, tcfg, "hostforms", tagImage, func(t *testing.T) {
		for _, name := range hostformsOrder {
			name := name
			fn := hostformsTests[name]
			t.Run(name, func(t *testing.T) {
				if fn == nil {
					t.Fatal("not implemented")
				}
				fn(t, ctx)
			})
		}
	})

	agentOrder := []string{
		"DXAC39_ClaudeCodeSetup",
		"DXAC41_PiSetup",
		"DXAC40_JWTNoLeak",
		"DXAC26_DenyRules",
		"DXAC42_CATrustConflict",
		"DXAC43_AgentSetupRefusals",
		"DXAC45_AgentIssue",
		"DXAC51_RemoteMode",
	}
	dxh.RunGroup(t, ctx, r, tcfg, "agent", tagImage, func(t *testing.T) {
		dxh.EnsureAgentSetup(t)
		for _, name := range agentOrder {
			name := name
			fn := agentTests[name]
			t.Run(name, func(t *testing.T) {
				if fn == nil {
					t.Fatal("not implemented")
				}
				fn(t, ctx)
			})
		}
	})
	dxh.ResetAgentSetup()

	bootagentOrder := []string{
		"DXAC46_BootstrapAgentSetup",
	}
	dxh.RunGroup(t, ctx, r, tcfg, "bootagent", tagImage, func(t *testing.T) {
		for _, name := range bootagentOrder {
			name := name
			fn := bootagentTests[name]
			t.Run(name, func(t *testing.T) {
				if fn == nil {
					t.Fatal("not implemented")
				}
				fn(t, ctx)
			})
		}
	})

	imageOrder := []string{
		"DXAC30_ImageSelection",
	}
	dxh.RunGroup(t, ctx, r, tcfg, "image", nil, func(t *testing.T) {
		for _, name := range imageOrder {
			name := name
			fn := imageTests[name]
			t.Run(name, func(t *testing.T) {
				if fn == nil {
					t.Fatal("not implemented")
				}
				fn(t, ctx)
			})
		}
	})
}

func testHarnessSkeleton(t *testing.T, ctx context.Context, groupDir, binDir string) {
	r := dxh.NewRunner()
	home := os.Getenv("HOME")

	uid := fmt.Sprintf("%d", os.Getuid())
	if out, err := r.Output(ctx, "launchctl", "print", "gui/"+uid); err != nil {
		t.Fatalf("launchctl print gui/%s: %v; output: %s", uid, err, out)
	}

	if err := checkFileWriteRemove(filepath.Join(home, "Library", "LaunchAgents")); err != nil {
		t.Fatalf("file write/remove in ~/Library/LaunchAgents: %v", err)
	}
	configBase := configBaseDir(home)
	if err := os.MkdirAll(configBase, 0o700); err != nil {
		t.Fatalf("mkdir ~/.config: %v", err)
	}
	if err := checkFileWriteRemove(configBase); err != nil {
		t.Fatalf("file write/remove in ~/.config: %v", err)
	}

	if err := checkListenerOnPort("127.0.0.1:8443"); err != nil {
		t.Fatalf("listener on 127.0.0.1:8443: %v", err)
	}

	if err := checkPTYRoundTrip(ctx, groupDir, binDir); err != nil {
		t.Fatalf("pseudo-terminal round trip: %v", err)
	}

	if err := dxh.CheckFreeSpace(ctx, r); err != nil {
		t.Fatalf("free space: %v", err)
	}

	gr := dxh.NewGroupRunner(groupDir, binDir)
	out, err := gr.Output(ctx, "sh", "-c", "echo $CLAUDE_CONFIG_DIR")
	if err != nil {
		t.Fatalf("child CLAUDE_CONFIG_DIR check: %v", err)
	}
	childCfgDir := strings.TrimSpace(string(out))
	parentCfgDir := os.Getenv("CLAUDE_CONFIG_DIR")
	if parentCfgDir != "" && childCfgDir == parentCfgDir {
		t.Fatalf("child inherited parent CLAUDE_CONFIG_DIR=%q", parentCfgDir)
	}
}

func cfgDir(home string) string {
	return filepath.Join(home, ".config", "sag")
}

func configBaseDir(home string) string {
	return filepath.Join(home, ".config")
}

func isPortHeld(addr string) bool {
	conn, err := net.DialTimeout("tcp", addr, time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

func pathExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func checkFileWriteRemove(dir string) error {
	f, err := os.CreateTemp(dir, "dxh-skeleton-*")
	if err != nil {
		return err
	}
	name := f.Name()
	f.Close()
	return os.Remove(name)
}

func checkListenerOnPort(addr string) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	return ln.Close()
}

func checkPTYRoundTrip(ctx context.Context, groupDir, binDir string) error {
	tty, err := dxh.StartTTY(ctx, groupDir, binDir, "echo", "dxh-pty-ok")
	if err != nil {
		return err
	}
	if err := tty.Expect(ctx, "dxh-pty-ok"); err != nil {
		return err
	}
	_, err = tty.Wait(ctx)
	return err
}
