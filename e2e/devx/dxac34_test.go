//go:build devxe2e

package devx_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kanthorlabs/sag/e2e/devx/dxh"
)

func init() {
	macTests["DXAC34_MacBootstrap"] = DXAC34_MacBootstrap
}

func DXAC34_MacBootstrap(t *testing.T, ctx context.Context) {
	t.Helper()

	home := os.Getenv("HOME")
	cfgDir := dxh.CfgDir(home)
	caPath := filepath.Join(cfgDir, "ca.pem")
	uid := fmt.Sprintf("%d", os.Getuid())
	r := dxh.NewRunner()

	out, err := runSAG(t, ctx, "bootstrap",
		"--autostart", "yes",
		"--agent-setup", "claude-code=no",
		"--agent-setup", "pi=no")
	if err != nil {
		t.Fatalf("DXAC34: bootstrap: %v\n%s", err, out)
	}
	outStr := string(out)

	if !strings.Contains(outStr, "ok podman") && !strings.Contains(outStr, "warn podman") {
		t.Fatalf("DXAC34: bootstrap output has no 'ok podman' or 'warn podman' line:\n%s", outStr)
	}
	if strings.Contains(outStr, "install podman") {
		t.Fatalf("DXAC34: bootstrap changed Podman; must not install it:\n%s", outStr)
	}

	answersData, err := os.ReadFile(filepath.Join(cfgDir, "answers.json"))
	if err != nil {
		t.Fatalf("DXAC34: read answers.json: %v", err)
	}
	var answers struct {
		PodmanPath string `json:"podman_path"`
	}
	if err := json.Unmarshal(answersData, &answers); err != nil {
		t.Fatalf("DXAC34: parse answers.json: %v", err)
	}
	if !filepath.IsAbs(answers.PodmanPath) {
		t.Fatalf("DXAC34: answers.json podman_path %q is not absolute", answers.PodmanPath)
	}

	if err := healthzOK(ctx, caPath); err != nil {
		t.Fatalf("DXAC34: GET /healthz: %v", err)
	}

	lOut, lErr := r.Output(ctx, "launchctl", "print", "gui/"+uid+"/net.kanthorlabs.sag")
	if lErr != nil {
		t.Fatalf("DXAC34: launchctl print gui/%s/net.kanthorlabs.sag: %v\n%s", uid, lErr, lOut)
	}

	if err := dxh.WaitLaunchAgentIdle(ctx, r, uid); err != nil {
		t.Fatalf("DXAC34: wait launch agent idle before service stop: %v", err)
	}

	stopOut, stopErr := runSAG(t, ctx, "service", "stop")
	if stopErr != nil {
		t.Fatalf("DXAC34: service stop: %v\n%s", stopErr, stopOut)
	}

	kickOut, kickErr := r.Output(ctx, "launchctl", "kickstart", "-k", "gui/"+uid+"/net.kanthorlabs.sag")
	if kickErr != nil {
		t.Fatalf("DXAC34: launchctl kickstart -k gui/%s/net.kanthorlabs.sag: %v\n%s", uid, kickErr, kickOut)
	}

	if err := healthzOK(ctx, caPath); err != nil {
		t.Fatalf("DXAC34: /healthz not 200 within 30s after kickstart -k: %v", err)
	}
}
