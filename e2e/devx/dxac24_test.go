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
	"regexp"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/kanthorlabs/sag/e2e/devx/dxh"
)

func init() {
	hostformsTests["DXAC24_HostJWTGenerate"] = func(t *testing.T, ctx context.Context) {
		DXAC24_HostJWTGenerate(t, ctx)
	}
}

func DXAC24_HostJWTGenerate(t *testing.T, ctx context.Context) {
	t.Helper()
	dxh.EnsureBootstrapped(t)

	uid := fmt.Sprintf("%d", os.Getuid())
	if err := dxh.WaitLaunchAgentIdle(ctx, dxh.NewRunner(), uid); err != nil {
		t.Fatalf("DXAC24: wait launch agent idle: %v", err)
	}

	code, out := runSAGWithInput(t, ctx,
		[]byte("postgres://u:p@127.0.0.1:1/db"),
		"credential", "create", "dxac24jwt",
		"--kind", "postgres",
		"--agent", "claude-code-dev",
		"--tls", "disable",
		"--stdin",
		"--accept-failed-check",
	)
	if code != 0 {
		t.Fatalf("DXAC24: credential create dxac24jwt: exit %d; output %s", code, out)
	}

	if err := dxh.WaitLaunchAgentIdle(ctx, dxh.NewRunner(), uid); err != nil {
		t.Fatalf("DXAC24: wait launch agent idle after create: %v", err)
	}
	if _, err := runSAG(t, ctx, "service", "stop"); err != nil {
		t.Fatalf("DXAC24: service stop: %v", err)
	}
	if _, err := runSAG(t, ctx, "service", "start"); err != nil {
		t.Fatalf("DXAC24: service start: %v", err)
	}

	sagBin, err := dxh.TestBinary()
	if err != nil {
		t.Fatalf("DXAC24: test binary: %v", err)
	}

	t.Run("no_terminal_exit3", func(t *testing.T) {
		code, combined := dxac24RunSetsid(t, ctx, sagBin, "jwt", "generate", "claude-code-dev")
		if strings.Contains(combined, "eyJ") {
			t.Fatalf("DXAC24-A: output contains a token; want none")
		}
		if code != 3 {
			t.Fatalf("DXAC24-A: exit = %d; want 3; output: %s", code, combined)
		}
	})

	t.Run("pty_redirect_exit3", func(t *testing.T) {
		outFile := filepath.Join(t.TempDir(), "jwt-redirect.txt")
		if err := os.WriteFile(outFile, nil, 0o600); err != nil {
			t.Fatalf("DXAC24-B: create output file: %v", err)
		}
		groupDir := t.TempDir()
		binDir := t.TempDir()
		script := sagBin + " jwt generate claude-code-dev > " + outFile + "; echo sag-exit-$?"
		tty, err := dxh.StartTTY(ctx, groupDir, binDir, "/bin/sh", "-c", script)
		if err != nil {
			t.Fatalf("DXAC24-B: start tty: %v", err)
		}
		if err := tty.Expect(ctx, "sag-exit-3"); err != nil {
			t.Fatalf("DXAC24-B: expected sag-exit-3 in output: %v; raw: %q", err, tty.RawOutput())
		}
		if _, err := tty.Wait(ctx); err != nil {
			t.Fatalf("DXAC24-B: wait: %v", err)
		}
		content, err := os.ReadFile(outFile)
		if err != nil {
			t.Fatalf("DXAC24-B: read output file: %v", err)
		}
		if len(content) != 0 {
			t.Fatalf("DXAC24-B: output file has %d bytes; want empty", len(content))
		}
	})

	t.Run("json_flag_exit2", func(t *testing.T) {
		code, combined := dxac24RunSetsid(t, ctx, sagBin, "jwt", "generate", "--json", "claude-code-dev")
		if strings.Contains(combined, "eyJ") {
			t.Fatalf("DXAC24-C: output contains a token; want none")
		}
		if code != 2 {
			t.Fatalf("DXAC24-C: exit = %d; want 2; output: %s", code, combined)
		}
	})

	t.Run("pty_prints_jwt", func(t *testing.T) {
		home := os.Getenv("HOME")
		cfgDir := dxh.CfgDir(home)
		caPEM, err := os.ReadFile(filepath.Join(cfgDir, "ca.pem"))
		if err != nil {
			t.Fatalf("DXAC24-D: read ca.pem: %v", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(caPEM) {
			t.Fatalf("DXAC24-D: ca.pem: no certificate found")
		}
		client := &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{RootCAs: pool},
			},
			Timeout: 5 * time.Second,
		}

		groupDir := t.TempDir()
		binDir := t.TempDir()
		tty, err := dxh.StartTTY(ctx, groupDir, binDir, sagBin, "jwt", "generate", "claude-code-dev")
		if err != nil {
			t.Fatalf("DXAC24-D: start tty: %v", err)
		}
		if err := tty.Expect(ctx, "\n"); err != nil {
			t.Fatalf("DXAC24-D: jwt generate did not print a line: %v", err)
		}
		raw := tty.RawOutput()
		jwtRe24 := regexp.MustCompile(`eyJ[A-Za-z0-9+/=_-]+\.[A-Za-z0-9+/=_-]+\.[A-Za-z0-9+/=_-]+`)
		jwt := jwtRe24.FindString(raw)
		if jwt != "" {
			tty.Redact(jwt)
		}
		code, err := tty.Wait(ctx)
		if err != nil {
			t.Fatalf("DXAC24-D: wait: %v", err)
		}
		if code != 0 {
			t.Fatalf("DXAC24-D: exit = %d; want 0", code)
		}
		if jwt == "" {
			t.Fatal("DXAC24-D: no JWT found in terminal output")
		}

		req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "https://localhost:8443/mcp", nil)
		req.Header.Set("Authorization", "Bearer "+jwt)
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("DXAC24-D: POST /mcp: %v", err)
		}
		resp.Body.Close()
		if resp.StatusCode == http.StatusUnauthorized {
			t.Fatal("DXAC24-D: POST /mcp: status 401; JWT not accepted")
		}
	})
}

func dxac24RunSetsid(t *testing.T, ctx context.Context, sagBin string, args ...string) (int, string) {
	t.Helper()
	groupDir := t.TempDir()
	binDir := t.TempDir()
	env, err := dxh.ChildEnv(os.Environ(), groupDir)
	if err != nil {
		t.Fatalf("ChildEnv: %v", err)
	}
	env = append(env, "PATH="+dxh.TestPATH(binDir))
	cmd := exec.CommandContext(ctx, sagBin, args...)
	cmd.Env = env
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	runErr := cmd.Run()
	code := 0
	if runErr != nil {
		if exitErr, ok := runErr.(*exec.ExitError); ok {
			code = exitErr.ExitCode()
		} else {
			t.Fatalf("dxac24RunSetsid(%v): %v", args, runErr)
		}
	}
	return code, buf.String()
}
