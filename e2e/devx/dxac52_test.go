//go:build devxe2e

package devx_test

import (
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

func init() {
	macTests["DXAC52_MacTailscale"] = func(t *testing.T, ctx context.Context) {
		DXAC52_MacTailscale(t, ctx)
	}
}

func DXAC52_MacTailscale(t *testing.T, ctx context.Context) {
	t.Helper()

	tailscalePath, err := exec.LookPath("tailscale")
	if err != nil {
		t.Fatal("DXAC52: prerequisite missing: tailscale not in PATH")
	}

	r := dxh.NewRunner()

	tsIP, err := dxh.ReadTailscaleIPv4(ctx, r)
	if err != nil {
		t.Fatalf("DXAC52: %v", err)
	}

	mask := func(s string) string { return dxh.MaskTailscaleIP(s, tsIP) }

	statusBefore, err := dxh.ReadServeStatusJSON(ctx, r)
	if err != nil {
		t.Fatalf("DXAC52: prerequisite missing: %v", err)
	}
	parsedBefore, err := dxh.ParseServeStatus(statusBefore)
	if err != nil {
		t.Fatalf("DXAC52: parse serve status: %v", err)
	}
	if dxh.HasTCP8443(parsedBefore) {
		t.Fatal("DXAC52: prerequisite missing: TCP 8443 entry already in tailscale serve status")
	}

	sagBin, err := dxh.TestBinary()
	if err != nil {
		t.Fatalf("DXAC52: test binary: %v", err)
	}

	podmanPath, err := exec.LookPath("podman")
	if err != nil {
		t.Fatalf("DXAC52: find podman: %v", err)
	}

	groupDir := t.TempDir()
	binDir := t.TempDir()

	if err := dxh.AddRealProgramLink(binDir, "podman", podmanPath); err != nil {
		t.Fatalf("DXAC52: link podman: %v", err)
	}
	if err := dxh.AddRealProgramLink(binDir, "tailscale", tailscalePath); err != nil {
		t.Fatalf("DXAC52: link tailscale: %v", err)
	}

	env, err := dxh.ChildEnv(os.Environ(), groupDir)
	if err != nil {
		t.Fatalf("DXAC52: child env: %v", err)
	}
	env = append(env, "PATH="+dxh.TestPATH(binDir))

	bsCtx, bsCancel := context.WithTimeout(ctx, 3*time.Minute)
	defer bsCancel()

	bsCmd := exec.CommandContext(bsCtx, sagBin, "bootstrap",
		"--autostart", "yes",
		"--agent-setup", "claude-code=no",
		"--agent-setup", "pi=no")
	bsCmd.Env = env
	var bsOut, bsErr strings.Builder
	bsCmd.Stdout = &bsOut
	bsCmd.Stderr = &bsErr
	bsRunErr := bsCmd.Run()
	bsCode := 0
	if bsRunErr != nil {
		if exitErr, ok := bsRunErr.(*exec.ExitError); ok {
			bsCode = exitErr.ExitCode()
		} else {
			t.Fatalf("DXAC52: run bootstrap: %v", bsRunErr)
		}
	}
	if bsCode != 0 {
		t.Fatalf("DXAC52: bootstrap exit %d; want 0\nstdout=%s\nstderr=%s",
			bsCode, mask(bsOut.String()), mask(bsErr.String()))
	}

	home := os.Getenv("HOME")
	cfgDir := dxh.CfgDir(home)

	statusAfterBS, err := dxh.ReadServeStatusJSON(ctx, r)
	if err != nil {
		t.Fatalf("DXAC52: read serve status after bootstrap: %v", err)
	}
	parsedAfterBS, err := dxh.ParseServeStatus(statusAfterBS)
	if err != nil {
		t.Fatalf("DXAC52: parse serve status after bootstrap: %v", err)
	}
	if !dxh.HasSAGRule(parsedAfterBS) {
		t.Fatal("DXAC52: SAG rule absent in tailscale serve status after bootstrap")
	}

	if err := dxac52CurlHealthz(ctx, cfgDir, tsIP); err != nil {
		t.Fatalf("DXAC52: curl https://<tailscale IP>:8443/healthz: %v", err)
	}

	if err := dxac52CheckNoWildcardListener(ctx); err != nil {
		t.Fatalf("DXAC52: lsof: %v", err)
	}

	if _, err := r.Output(ctx, "tailscale", "serve", "--tcp", "8443", "off"); err != nil {
		t.Fatalf("DXAC52: tailscale serve --tcp 8443 off: %v", err)
	}

	statusAfterOff, err := dxh.ReadServeStatusJSON(ctx, r)
	if err != nil {
		t.Fatalf("DXAC52: read serve status after off: %v", err)
	}
	parsedAfterOff, err := dxh.ParseServeStatus(statusAfterOff)
	if err != nil {
		t.Fatalf("DXAC52: parse serve status after off: %v", err)
	}
	if dxh.HasTCP8443(parsedAfterOff) {
		t.Fatal("DXAC52: TCP 8443 entry still in serve status after tailscale serve --tcp 8443 off")
	}

	curlErr := dxac52CurlHealthz(ctx, cfgDir, tsIP)
	if curlErr == nil {
		t.Fatal("DXAC52: curl succeeded after tailscale serve --tcp 8443 off; want failure")
	}

	if err := dxh.EqualServeStatus(statusBefore, statusAfterOff); err != nil {
		t.Fatalf("DXAC52: serve status after off != before: %v", err)
	}
}

func dxac52CurlHealthz(ctx context.Context, cfgDir, tsIP string) error {
	caPEM, err := os.ReadFile(filepath.Join(cfgDir, "ca.pem"))
	if err != nil {
		return fmt.Errorf("read ca.pem: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		return fmt.Errorf("ca.pem: no certificate found")
	}
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{RootCAs: pool},
		},
		Timeout: 5 * time.Second,
	}
	url := "https://" + tsIP + ":8443/healthz"
	reqCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("status %d; want 200", resp.StatusCode)
	}
	return nil
}

func dxac52CheckNoWildcardListener(ctx context.Context) error {
	lsofCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(lsofCtx, "lsof", "-nP", "-iTCP:8443", "-sTCP:LISTEN").Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return nil
		}
		return fmt.Errorf("lsof: %w", err)
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.Contains(line, "*:8443") {
			return fmt.Errorf("lsof shows *:8443 listener: %s", line)
		}
	}
	return nil
}
