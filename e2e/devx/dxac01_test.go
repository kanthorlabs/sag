//go:build devxe2e

package devx_test

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
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

	out, err := runSAG(t, ctx, "bootstrap",
		"--autostart", "yes",
		"--agent-setup", "claude-code=no",
		"--agent-setup", "pi=no")
	if err != nil {
		t.Fatalf("bootstrap --autostart yes: %v\n%s", err, out)
	}

	home := os.Getenv("HOME")
	cfgDir := dxh.CfgDir(home)

	caPath := filepath.Join(cfgDir, "ca.pem")
	if err := healthzOK(ctx, caPath); err != nil {
		t.Fatalf("GET /healthz: %v", err)
	}

	r := dxh.NewRunner()
	uid := fmt.Sprintf("%d", os.Getuid())
	lOut, lErr := r.Output(ctx, "launchctl", "print", "gui/"+uid+"/net.kanthorlabs.sag")
	if lErr != nil {
		t.Fatalf("launchctl print gui/%s/net.kanthorlabs.sag: %v\n%s", uid, lErr, lOut)
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

func waitLaunchAgentIdle(ctx context.Context, uid string) error {
	r := dxh.NewRunner()
	deadline := time.Now().Add(90 * time.Second)
	for {
		out, err := r.Output(ctx, "launchctl", "print", "gui/"+uid+"/net.kanthorlabs.sag")
		if err != nil || strings.Contains(string(out), "state = not running") {
			return nil
		}
		if time.Now().After(deadline) {
			var state []string
			for _, l := range strings.Split(string(out), "\n") {
				l = strings.TrimSpace(l)
				if strings.HasPrefix(l, "state = ") || strings.HasPrefix(l, "last exit code = ") || strings.HasPrefix(l, "runs = ") || strings.HasPrefix(l, "pid = ") {
					state = append(state, l)
				}
			}
			return fmt.Errorf("launch agent not idle after 90s: %s", strings.Join(state, "; "))
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
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

func runSAG(t *testing.T, ctx context.Context, args ...string) ([]byte, error) {
	t.Helper()
	bin, err := dxh.TestBinary()
	if err != nil {
		return nil, err
	}
	out, err := dxh.NewProductRunner(t).Output(ctx, bin, args...)
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return out, fmt.Errorf("%w; stderr: %s", err, exitErr.Stderr)
	}
	return out, err
}

func containerRunning(ctx context.Context) bool {
	r := dxh.NewRunner()
	out, err := r.Output(ctx, "podman", "container", "inspect", "sag", "--format", "{{.State.Running}}")
	return err == nil && strings.TrimSpace(string(out)) == "true"
}
