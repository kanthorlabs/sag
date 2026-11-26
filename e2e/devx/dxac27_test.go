//go:build devxe2e

package devx_test

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/kanthorlabs/sag/e2e/devx/dxh"
)

func DXAC27_Autostart(t *testing.T, ctx context.Context) {
	t.Helper()
	home := os.Getenv("HOME")
	uid := fmt.Sprintf("%d", os.Getuid())

	out, err := runSAG(ctx, "bootstrap",
		"--autostart", "no",
		"--agent-setup", "claude-code=no",
		"--agent-setup", "pi=no")
	if err != nil {
		t.Fatalf("bootstrap --autostart no: %v\n%s", err, out)
	}

	if plistExists(home) {
		t.Fatal("LaunchAgent plist exists after --autostart no")
	}
	if launchAgentShows(ctx, uid) {
		t.Fatal("launchctl shows agent after --autostart no")
	}

	out, err = runSAG(ctx, "bootstrap",
		"--autostart", "yes",
		"--agent-setup", "claude-code=no",
		"--agent-setup", "pi=no")
	if err != nil {
		t.Fatalf("bootstrap --autostart yes: %v\n%s", err, out)
	}

	if !plistExists(home) {
		t.Fatal("LaunchAgent plist missing after --autostart yes")
	}

	content, err := plistContent(home)
	if err != nil {
		t.Fatalf("read plist: %v", err)
	}
	if !strings.Contains(content, "<true/>") {
		t.Fatal("plist does not have RunAtLoad true")
	}

	if !launchAgentShows(ctx, uid) {
		t.Fatal("launchctl does not show agent after --autostart yes")
	}

	r := dxh.NewRunner()

	_, err = runSAG(ctx, "service", "stop")
	if err != nil {
		t.Fatalf("service stop: %v", err)
	}

	kickOut, err := r.Output(ctx, "launchctl", "kickstart", "-k",
		"gui/"+uid+"/net.kanthorlabs.sag")
	if err != nil {
		t.Fatalf("launchctl kickstart: %v\n%s", err, kickOut)
	}

	cfgDir := dxh.CfgDir(home)
	caPath := cfgDir + "/ca.pem"
	if err := healthzOK(ctx, caPath); err != nil {
		t.Fatalf("healthz after kickstart: %v", err)
	}
}
