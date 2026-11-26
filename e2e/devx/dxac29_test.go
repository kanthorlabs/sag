//go:build devxe2e

package devx_test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kanthorlabs/sag/e2e/devx/dxh"
)

func init() {
	certTests["DXAC29_CertRenewal"] = func(t *testing.T, ctx context.Context) {
		DXAC29_CertRenewal(t, ctx)
	}
}

func DXAC29_CertRenewal(t *testing.T, ctx context.Context) {
	t.Helper()

	home := os.Getenv("HOME")
	cfgDir := dxh.CfgDir(home)
	caPath := filepath.Join(cfgDir, "ca.pem")
	caNewPath := filepath.Join(cfgDir, "ca.pem.new")
	uid := fmt.Sprintf("%d", os.Getuid())

	t.Setenv("SAG_TEST_CERT_DAYS", "20")

	t.Log("step 1: bootstrap with 20-day cert")
	out, err := runSAG(t, ctx,
		"bootstrap",
		"--autostart", "yes",
		"--agent-setup", "claude-code=no",
		"--agent-setup", "pi=no")
	if err != nil {
		t.Fatalf("bootstrap (step 1): %v\n%s", err, out)
	}
	if err := healthzOK(ctx, caPath); err != nil {
		t.Fatalf("healthz after bootstrap (step 1): %v", err)
	}

	t.Log("step 2: credential list must write expiry warn to stderr")
	if err := waitLaunchAgentIdle(ctx, uid); err != nil {
		t.Fatalf("waitLaunchAgentIdle (step 2): %v", err)
	}
	credStdout, credStderr := runSAGWithStderr(t, ctx, "credential", "list")
	_ = credStdout
	if !strings.Contains(credStderr, "warn") || !strings.Contains(credStderr, "expires") {
		t.Fatalf("credential list (step 2): want expiry warn in stderr, got stderr=%q", credStderr)
	}
	if !strings.Contains(credStderr, "sag bootstrap") {
		t.Fatalf("credential list (step 2): want 'sag bootstrap' in warn, got stderr=%q", credStderr)
	}

	t.Log("step 3: bootstrap replaces ca.pem")
	caDataBefore, err := os.ReadFile(caPath)
	if err != nil {
		t.Fatalf("read ca.pem before renewal: %v", err)
	}
	if err := waitLaunchAgentIdle(ctx, uid); err != nil {
		t.Fatalf("waitLaunchAgentIdle (step 3): %v", err)
	}
	out, err = runSAG(t, ctx,
		"bootstrap",
		"--autostart", "yes",
		"--agent-setup", "claude-code=no",
		"--agent-setup", "pi=no")
	if err != nil {
		t.Fatalf("bootstrap (step 3): %v\n%s", err, out)
	}
	caDataAfter, err := os.ReadFile(caPath)
	if err != nil {
		t.Fatalf("read ca.pem after renewal: %v", err)
	}
	if bytes.Equal(caDataBefore, caDataAfter) {
		t.Fatal("ca.pem was not replaced by bootstrap (step 3)")
	}
	if err := healthzOK(ctx, caPath); err != nil {
		t.Fatalf("healthz with new ca.pem (step 3): %v", err)
	}
	if _, err := os.Stat(caNewPath); err == nil {
		t.Fatal("ca.pem.new must not exist after successful renewal (step 3)")
	}

	t.Log("step 4: hook stops renewal after step 3 of section 5.7")
	t.Setenv("SAG_TEST_FAIL_CERT_RENEWAL", "1")
	if err := waitLaunchAgentIdle(ctx, uid); err != nil {
		t.Fatalf("waitLaunchAgentIdle (step 4): %v", err)
	}
	_, hookErr := runSAG(t, ctx,
		"bootstrap",
		"--autostart", "yes",
		"--agent-setup", "claude-code=no",
		"--agent-setup", "pi=no")
	if hookErr == nil {
		t.Fatal("bootstrap with hook (step 4): want non-zero exit, got exit 0")
	}
	if _, err := os.Stat(caNewPath); err != nil {
		t.Fatalf("ca.pem.new must exist after hook-stopped renewal (step 4): %v", err)
	}

	t.Log("step 5: next bootstrap exits 0, removes ca.pem.new, healthz 200")
	os.Unsetenv("SAG_TEST_FAIL_CERT_RENEWAL")
	if err := waitLaunchAgentIdle(ctx, uid); err != nil {
		t.Fatalf("waitLaunchAgentIdle (step 5): %v", err)
	}
	out, err = runSAG(t, ctx,
		"bootstrap",
		"--autostart", "yes",
		"--agent-setup", "claude-code=no",
		"--agent-setup", "pi=no")
	if err != nil {
		t.Fatalf("bootstrap after hook (step 5): %v\n%s", err, out)
	}
	if _, err := os.Stat(caNewPath); err == nil {
		t.Fatal("ca.pem.new must be removed by bootstrap (step 5)")
	}
	if err := healthzOK(ctx, caPath); err != nil {
		t.Fatalf("healthz with final ca.pem (step 5): %v", err)
	}
}

func runSAGWithStderr(t *testing.T, ctx context.Context, args ...string) (stdout, stderr string) {
	t.Helper()
	bin, err := dxh.TestBinary()
	if err != nil {
		t.Fatalf("TestBinary: %v", err)
	}

	groupDir := t.TempDir()
	binDir := t.TempDir()
	env, err := dxh.ChildEnv(os.Environ(), groupDir)
	if err != nil {
		t.Fatalf("ChildEnv: %v", err)
	}
	env = append(env, "PATH="+dxh.TestPATH(binDir))

	var stdoutBuf, stderrBuf bytes.Buffer
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = env
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf
	_ = cmd.Run()
	return stdoutBuf.String(), stderrBuf.String()
}
