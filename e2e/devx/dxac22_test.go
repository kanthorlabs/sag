//go:build devxe2e

package devx_test

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/kanthorlabs/sag/e2e/devx/dxh"
)

func init() {
	deleteTests["DXAC22_DeleteResume"] = func(t *testing.T, ctx context.Context) {
		DXAC22_DeleteResume(t, ctx)
	}
}

func DXAC22_DeleteResume(t *testing.T, ctx context.Context) {
	t.Helper()
	dxh.EnsureCanary(t)

	uid := fmt.Sprintf("%d", os.Getuid())
	r := dxh.NewRunner()
	if err := dxh.WaitLaunchAgentIdle(ctx, r, uid); err != nil {
		t.Fatalf("DXAC22: wait launch agent idle: %v", err)
	}

	port, err := canaryPGPort(ctx, r)
	if err != nil {
		t.Fatalf("DXAC22: canary PG port: %v", err)
	}

	canary := dxh.ActiveCanaryParts
	dsn := (&url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(canary.User, canary.Password),
		Host:   "host.containers.internal:" + port,
		Path:   "/" + canary.Database,
	}).String()

	const name = "dxac22-pg"

	code, _ := runSAGWithInput(t, ctx, []byte(dsn),
		"credential", "create", name,
		"--kind", "postgres",
		"--agent", "pi",
		"--tls", "disable",
		"--stdin",
	)
	if code != 0 {
		t.Fatalf("DXAC22: create %s: exit %d", name, code)
	}

	if err := dxh.WaitLaunchAgentIdle(ctx, r, uid); err != nil {
		t.Fatalf("DXAC22: wait launch agent idle after create: %v", err)
	}

	t.Setenv("SAG_TEST_FAIL_CRED_DELETE", "1")

	code, tty := deleteViaStartTTY(t, ctx, name)
	if code == 0 {
		t.Fatalf("DXAC22: first delete (hook enabled): exit 0; want non-zero; output: %s", tty.RawOutput())
	}
	if !strings.Contains(tty.RawOutput(), "test hook") {
		t.Fatalf("DXAC22: first delete output does not contain hook message; output: %s", tty.RawOutput())
	}

	if !secretExists(ctx, t, "sag-res-"+name+".conn") {
		t.Fatalf("DXAC22: conn secret missing after first (failed) delete; hook should not have deleted it")
	}

	if err := dxh.WaitLaunchAgentIdle(ctx, r, uid); err != nil {
		t.Fatalf("DXAC22: wait launch agent idle after first delete: %v", err)
	}

	os.Unsetenv("SAG_TEST_FAIL_CRED_DELETE")

	code, tty = deleteViaStartTTY(t, ctx, name)
	if code != 0 {
		t.Fatalf("DXAC22: second delete: exit %d; want 0; output: %s", code, tty.RawOutput())
	}

	if secretExists(ctx, t, "sag-res-"+name+".conn") {
		t.Fatalf("DXAC22: conn secret still exists after second delete")
	}
}
