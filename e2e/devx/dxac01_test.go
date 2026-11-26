//go:build devxe2e

package devx_test

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
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

func DXAC01_Bootstrap(t *testing.T, ctx context.Context) {
	t.Helper()
	dxh.EnsureBootstrapped(t)

	home := os.Getenv("HOME")
	cfgDir := dxh.CfgDir(home)

	caPath := filepath.Join(cfgDir, "ca.pem")
	if err := healthzOK(ctx, caPath); err != nil {
		t.Fatalf("GET /healthz: %v", err)
	}

	r := dxh.NewRunner()
	uid := fmt.Sprintf("%d", os.Getuid())
	out, err := r.Output(ctx, "launchctl", "print", "gui/"+uid+"/net.kanthorlabs.sag")
	if err != nil {
		t.Fatalf("launchctl print gui/%s/net.kanthorlabs.sag: %v\n%s", uid, err, out)
	}
}

func healthzOK(ctx context.Context, caPath string) error {
	pem, err := os.ReadFile(caPath)
	if err != nil {
		return err
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pem) {
		return fmt.Errorf("ca.pem: no certificate found")
	}
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{RootCAs: pool},
		},
		Timeout: 5 * time.Second,
	}

	deadline := time.Now().Add(30 * time.Second)
	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://localhost:8443/healthz", nil)
		if err != nil {
			return err
		}
		resp, err := client.Do(req)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		if time.Now().After(deadline) {
			if err != nil {
				return fmt.Errorf("healthz timeout: %w", err)
			}
			return fmt.Errorf("healthz timeout: status %d", resp.StatusCode)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
}

func launchAgentShows(ctx context.Context, uid string) bool {
	r := dxh.NewRunner()
	_, err := r.Output(ctx, "launchctl", "print", "gui/"+uid+"/net.kanthorlabs.sag")
	return err == nil
}

func plistExists(home string) bool {
	p := filepath.Join(home, "Library", "LaunchAgents", "net.kanthorlabs.sag.plist")
	_, err := os.Stat(p)
	return err == nil
}

func plistContent(home string) (string, error) {
	p := filepath.Join(home, "Library", "LaunchAgents", "net.kanthorlabs.sag.plist")
	b, err := os.ReadFile(p)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func healthzFails(ctx context.Context, caPath string) bool {
	err := healthzOK(ctx, caPath)
	return err != nil
}

func sagBinForTest() (string, error) {
	if bin := os.Getenv("SAG_TEST_BIN"); bin != "" {
		return bin, nil
	}
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", fmt.Errorf("git rev-parse --show-toplevel: %w", err)
	}
	return filepath.Join(string(bytes.TrimSpace(out)), "bin", "sag-devxtest"), nil
}

func runSAG(ctx context.Context, args ...string) ([]byte, error) {
	bin, err := sagBinForTest()
	if err != nil {
		return nil, err
	}
	r := dxh.NewRunner()
	return r.Output(ctx, bin, args...)
}

func containerRunning(ctx context.Context) bool {
	r := dxh.NewRunner()
	out, err := r.Output(ctx, "podman", "container", "inspect", "sag", "--format", "{{.State.Running}}")
	return err == nil && strings.TrimSpace(string(out)) == "true"
}
