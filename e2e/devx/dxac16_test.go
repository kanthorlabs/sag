//go:build devxe2e

package devx_test

import (
	"bytes"
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kanthorlabs/sag/e2e/devx/dxh"
)

func init() {
	credentialTests["DXAC16_CreateRollback"] = func(t *testing.T, ctx context.Context) {
		DXAC16_CreateRollback(t, ctx)
	}
}

func DXAC16_CreateRollback(t *testing.T, ctx context.Context) {
	t.Helper()
	dxh.EnsureCanary(t)

	uid := fmt.Sprintf("%d", os.Getuid())
	if err := dxh.WaitLaunchAgentIdle(ctx, dxh.NewRunner(), uid); err != nil {
		t.Fatalf("DXAC16: wait launch agent idle: %v", err)
	}

	r := dxh.NewRunner()
	port, err := canaryPGPort(ctx, r)
	if err != nil {
		t.Fatalf("DXAC16: get canary PG port: %v", err)
	}

	canary := dxh.ActiveCanaryParts
	dsn := (&url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(canary.User, canary.Password),
		Host:   "host.containers.internal:" + port,
		Path:   "/" + canary.Database,
	}).String()

	const name = "mydb-16"
	connSecret := "sag-res-" + name + ".conn"

	home := os.Getenv("HOME")
	sagYAML := filepath.Join(dxh.CfgDir(home), "sag.yaml")
	yamlBefore, err := os.ReadFile(sagYAML)
	if err != nil {
		t.Fatalf("DXAC16: read sag.yaml before: %v", err)
	}

	code, combined := runSAGWithInputAndEnv(t, ctx, []byte(dsn),
		[]string{"SAG_TEST_FAIL_CRED_CREATE=1"},
		"credential", "create", name,
		"--kind", "postgres",
		"--agent", "pi",
		"--stdin",
		"--tls", "disable",
	)
	if code != 1 {
		t.Fatalf("DXAC16: forced rollback: exit %d; want 1", code)
	}
	if !strings.Contains(combined, "test hook: fail at step 3") {
		t.Fatalf("DXAC16: output does not contain hook message")
	}

	if secretExists(ctx, t, connSecret) {
		t.Fatalf("DXAC16: rollback left secret %s behind", connSecret)
	}

	yamlAfter, err := os.ReadFile(sagYAML)
	if err != nil {
		t.Fatalf("DXAC16: read sag.yaml after: %v", err)
	}
	if !bytes.Equal(yamlBefore, yamlAfter) {
		t.Fatalf("DXAC16: sag.yaml changed after rollback")
	}

	caPath := filepath.Join(dxh.CfgDir(home), "ca.pem")
	if err := healthzOK(ctx, caPath); err != nil {
		t.Fatalf("DXAC16: /healthz after rollback: %v", err)
	}
}

func runSAGWithInputAndEnv(t *testing.T, ctx context.Context, stdin []byte, extraEnv []string, args ...string) (int, string) {
	t.Helper()
	bin, err := dxh.TestBinary()
	if err != nil {
		t.Fatalf("TestBinary: %v", err)
	}
	groupDir := t.TempDir()
	binDir := t.TempDir()
	baseEnv := append(os.Environ(), extraEnv...)
	env, err := dxh.ChildEnv(baseEnv, groupDir)
	if err != nil {
		t.Fatalf("ChildEnv: %v", err)
	}
	env = append(env, "PATH="+dxh.TestPATH(binDir))

	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = env
	cmd.Stdin = bytes.NewReader(stdin)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	runErr := cmd.Run()
	code := 0
	if runErr != nil {
		if exitErr, ok := runErr.(*exec.ExitError); ok {
			code = exitErr.ExitCode()
		} else {
			t.Fatalf("runSAGWithInputAndEnv(%v): %v", args, runErr)
		}
	}
	return code, out.String()
}
