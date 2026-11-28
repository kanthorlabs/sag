//go:build devxe2e

package devx_test

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/kanthorlabs/sag/e2e/devx/dxh"
)

func DXAC27_Autostart(t *testing.T, ctx context.Context) {
	t.Helper()
	home := os.Getenv("HOME")
	uid := fmt.Sprintf("%d", os.Getuid())

	t.Logf("step 1 (bootstrap yes) at %s", time.Now().Format(time.RFC3339))
	out, err := runSAG(t, ctx, "bootstrap",
		"--autostart", "yes",
		"--agent-setup", "claude-code=no",
		"--agent-setup", "pi=no")
	if err != nil {
		t.Fatalf("bootstrap --autostart yes (step 1): %v\n%s", err, out)
	}

	if runtime.GOOS == "linux" {
		if !quadletExists(home) {
			t.Fatal("Quadlet file missing after --autostart yes (step 1)")
		}
		qc, err := quadletContent(home)
		if err != nil {
			t.Fatalf("read quadlet (step 1): %v", err)
		}
		if !strings.Contains(qc, "[Install]") || !strings.Contains(qc, "WantedBy=default.target") {
			t.Fatal("Quadlet missing [Install] WantedBy=default.target after --autostart yes (step 1)")
		}
		if !systemdUnitInDefaultTarget(ctx) {
			t.Fatal("sag.service not in default.target after --autostart yes (step 1)")
		}
	} else {
		if !plistExists(home) {
			t.Fatal("LaunchAgent plist missing after --autostart yes (step 1)")
		}
		content, err := plistContent(home)
		if err != nil {
			t.Fatalf("read plist (step 1): %v", err)
		}
		if !runAtLoadTrue(content) {
			t.Fatal("plist RunAtLoad is not true after --autostart yes (step 1)")
		}
		if !launchAgentShows(ctx, uid) {
			t.Fatal("launchctl does not show agent after --autostart yes (step 1)")
		}
	}

	t.Logf("step 2 (bootstrap no) at %s", time.Now().Format(time.RFC3339))
	out, err = runSAG(t, ctx, "bootstrap",
		"--autostart", "no",
		"--agent-setup", "claude-code=no",
		"--agent-setup", "pi=no")
	if err != nil {
		t.Fatalf("bootstrap --autostart no (step 2): %v\n%s", err, out)
	}

	if runtime.GOOS == "linux" {
		if !quadletExists(home) {
			t.Fatal("Quadlet file missing after --autostart no (step 2); bootstrap must still write it")
		}
		qc, err := quadletContent(home)
		if err != nil {
			t.Fatalf("read quadlet (step 2): %v", err)
		}
		if strings.Contains(qc, "[Install]") {
			t.Fatal("Quadlet has [Install] section after --autostart no (step 2)")
		}
		if systemdUnitInDefaultTarget(ctx) {
			t.Fatal("sag.service in default.target after --autostart no (step 2)")
		}
	} else {
		if plistExists(home) {
			t.Fatal("LaunchAgent plist exists after --autostart no (step 2)")
		}
		if launchAgentShows(ctx, uid) {
			t.Fatal("launchctl shows agent after --autostart no (step 2)")
		}
	}

	t.Logf("step 3 (bootstrap yes) at %s", time.Now().Format(time.RFC3339))
	out, err = runSAG(t, ctx, "bootstrap",
		"--autostart", "yes",
		"--agent-setup", "claude-code=no",
		"--agent-setup", "pi=no")
	if err != nil {
		t.Fatalf("bootstrap --autostart yes (step 3): %v\n%s", err, out)
	}

	if runtime.GOOS == "linux" {
		if !quadletExists(home) {
			t.Fatal("Quadlet file missing after --autostart yes (step 3)")
		}
		qc, err := quadletContent(home)
		if err != nil {
			t.Fatalf("read quadlet (step 3): %v", err)
		}
		if !strings.Contains(qc, "[Install]") || !strings.Contains(qc, "WantedBy=default.target") {
			t.Fatal("Quadlet missing [Install] WantedBy=default.target after --autostart yes (step 3)")
		}
		if !systemdUnitInDefaultTarget(ctx) {
			t.Fatal("sag.service not in default.target after --autostart yes (step 3)")
		}
	} else {
		if !plistExists(home) {
			t.Fatal("LaunchAgent plist missing after --autostart yes (step 3)")
		}
		content, err := plistContent(home)
		if err != nil {
			t.Fatalf("read plist (step 3): %v", err)
		}
		if !runAtLoadTrue(content) {
			t.Fatal("plist RunAtLoad is not true after --autostart yes (step 3)")
		}
		if !launchAgentShows(ctx, uid) {
			t.Fatal("launchctl does not show agent after --autostart yes (step 3)")
		}
	}

	t.Logf("step 4 (service stop) at %s", time.Now().Format(time.RFC3339))
	out, err = runSAG(t, ctx, "service", "stop")
	if err != nil {
		t.Fatalf("service stop: %v\n%s", err, out)
	}

	if containerRunning(ctx) {
		t.Fatal("container running before restart; want stopped")
	}

	r := dxh.NewRunner()
	cfgDir := dxh.CfgDir(home)
	caPath := cfgDir + "/ca.pem"

	if runtime.GOOS == "linux" {
		t.Logf("step 5 (systemctl start) at %s", time.Now().Format(time.RFC3339))
		startOut, err := r.Output(ctx, "systemctl", "--user", "start", "sag")
		if err != nil {
			t.Fatalf("systemctl --user start sag: %v\n%s", err, startOut)
		}
		if err := healthzOK(ctx, caPath); err != nil {
			t.Fatalf("healthz after systemctl start: %v", err)
		}
	} else {
		t.Logf("step 5 (kickstart) at %s", time.Now().Format(time.RFC3339))
		kickOut, err := r.Output(ctx, "launchctl", "kickstart", "-k",
			"gui/"+uid+"/net.kanthorlabs.sag")
		if err != nil {
			t.Fatalf("launchctl kickstart: %v\n%s", err, kickOut)
		}
		if err := healthzOK(ctx, caPath); err != nil {
			t.Fatalf("healthz after kickstart: %v", err)
		}
	}
}

func runAtLoadTrue(content string) bool {
	idx := strings.Index(content, "<key>RunAtLoad</key>")
	if idx < 0 {
		return false
	}
	after := strings.TrimSpace(content[idx+len("<key>RunAtLoad</key>"):])
	return strings.HasPrefix(after, "<true/>")
}

func healthzFailsOnce(ctx context.Context, caPath string) bool {
	pemBytes, err := os.ReadFile(caPath)
	if err != nil {
		return true
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pemBytes) {
		return true
	}
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{RootCAs: pool},
		},
		Timeout: 2 * time.Second,
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://localhost:8443/healthz", nil)
	if err != nil {
		return true
	}
	resp, err := client.Do(req)
	if err != nil {
		return true
	}
	resp.Body.Close()
	return resp.StatusCode != http.StatusOK
}
