//go:build devxe2e

package devx_test

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

	resolvedPodman, err := exec.LookPath("podman")
	if err != nil {
		t.Fatalf("DXAC34: resolve podman path before bootstrap: %v", err)
	}
	podmanVerBefore, err := exec.CommandContext(ctx, resolvedPodman, "--version").Output()
	if err != nil {
		t.Fatalf("DXAC34: podman --version before bootstrap: %v", err)
	}
	statBefore, err := os.Stat(resolvedPodman)
	if err != nil {
		t.Fatalf("DXAC34: stat podman before bootstrap: %v", err)
	}

	out, err := runSAG(t, ctx, "bootstrap",
		"--autostart", "yes",
		"--agent-setup", "claude-code=no",
		"--agent-setup", "pi=no")
	if err != nil {
		t.Fatalf("DXAC34: bootstrap: %v\n%s", err, out)
	}
	outStr := string(out)

	podmanLineFound := false
	for _, line := range strings.Split(outStr, "\n") {
		if strings.HasPrefix(line, "ok podman") || strings.HasPrefix(line, "warn podman") {
			podmanLineFound = true
		}
		if strings.HasPrefix(line, "install podman") {
			t.Fatalf("DXAC34: bootstrap output has 'install podman' line; must not install Podman:\n%s", outStr)
		}
	}
	if !podmanLineFound {
		t.Fatalf("DXAC34: bootstrap output has no 'ok podman' or 'warn podman' line:\n%s", outStr)
	}

	resolvedPodmanAfter, err := exec.LookPath("podman")
	if err != nil {
		t.Fatalf("DXAC34: resolve podman path after bootstrap: %v", err)
	}
	if resolvedPodmanAfter != resolvedPodman {
		t.Fatalf("DXAC34: podman path changed: %s → %s", resolvedPodman, resolvedPodmanAfter)
	}
	podmanVerAfter, err := exec.CommandContext(ctx, resolvedPodman, "--version").Output()
	if err != nil {
		t.Fatalf("DXAC34: podman --version after bootstrap: %v", err)
	}
	if string(podmanVerAfter) != string(podmanVerBefore) {
		t.Fatalf("DXAC34: podman --version changed: %q → %q", podmanVerBefore, podmanVerAfter)
	}
	statAfter, err := os.Stat(resolvedPodman)
	if err != nil {
		t.Fatalf("DXAC34: stat podman after bootstrap: %v", err)
	}
	if statAfter.Size() != statBefore.Size() || statAfter.ModTime() != statBefore.ModTime() {
		t.Fatalf("DXAC34: podman binary changed: size %d→%d mtime %s→%s",
			statBefore.Size(), statAfter.Size(),
			statBefore.ModTime().Format(time.RFC3339Nano),
			statAfter.ModTime().Format(time.RFC3339Nano))
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
	if answers.PodmanPath != resolvedPodman {
		t.Fatalf("DXAC34: answers.json podman_path %q != resolved path %q", answers.PodmanPath, resolvedPodman)
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

	if err := waitHealthzDown(caPath, 15*time.Second); err != nil {
		t.Fatalf("DXAC34: /healthz still 200 after service stop; expect connection refused: %v", err)
	}

	kickStart := time.Now()
	kickOut, kickErr := r.Output(ctx, "launchctl", "kickstart", "-k", "gui/"+uid+"/net.kanthorlabs.sag")
	if kickErr != nil {
		t.Fatalf("DXAC34: launchctl kickstart -k gui/%s/net.kanthorlabs.sag: %v\n%s", uid, kickErr, kickOut)
	}

	kickCtx, cancel := context.WithDeadline(ctx, kickStart.Add(30*time.Second))
	defer cancel()
	if err := healthzOK(kickCtx, caPath); err != nil {
		t.Fatalf("DXAC34: /healthz not 200 within 30s after kickstart -k: %v", err)
	}
}

func waitHealthzDown(caPath string, timeout time.Duration) error {
	pem, err := os.ReadFile(caPath)
	if err != nil {
		return nil
	}
	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(pem)
	client := &http.Client{
		Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool}},
		Timeout:   2 * time.Second,
	}
	deadline := time.Now().Add(timeout)
	for {
		resp, err := client.Get("https://localhost:8443/healthz")
		if err != nil {
			return nil
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("service still up after %s", timeout)
		}
		time.Sleep(500 * time.Millisecond)
	}
}
