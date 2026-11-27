//go:build devxe2e

package devx_test

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"testing"

	"github.com/kanthorlabs/sag/e2e/devx/dxh"
)

func init() {
	deleteTests["DXAC21_DeleteStopped"] = func(t *testing.T, ctx context.Context) {
		DXAC21_DeleteStopped(t, ctx)
	}
}

func DXAC21_DeleteStopped(t *testing.T, ctx context.Context) {
	t.Helper()
	dxh.EnsureCanary(t)

	uid := fmt.Sprintf("%d", os.Getuid())
	r := dxh.NewRunner()
	if err := dxh.WaitLaunchAgentIdle(ctx, r, uid); err != nil {
		t.Fatalf("DXAC21: wait launch agent idle: %v", err)
	}

	port, err := canaryPGPort(ctx, r)
	if err != nil {
		t.Fatalf("DXAC21: canary PG port: %v", err)
	}

	canary := dxh.ActiveCanaryParts
	dsn := (&url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(canary.User, canary.Password),
		Host:   "host.containers.internal:" + port,
		Path:   "/" + canary.Database,
	}).String()

	const name = "dxac21-pg"

	code, _ := runSAGWithInput(t, ctx, []byte(dsn),
		"credential", "create", name,
		"--kind", "postgres",
		"--agent", "pi",
		"--tls", "disable",
		"--stdin",
	)
	if code != 0 {
		t.Fatalf("DXAC21: create %s: exit %d", name, code)
	}

	if err := dxh.WaitLaunchAgentIdle(ctx, r, uid); err != nil {
		t.Fatalf("DXAC21: wait launch agent idle after create: %v", err)
	}

	out, err := runSAG(t, ctx, "service", "stop")
	if err != nil {
		t.Fatalf("DXAC21: service stop: %v\n%s", err, out)
	}

	t.Cleanup(func() {
		if _, err := runSAG(t, ctx, "service", "start"); err != nil {
			t.Logf("DXAC21: cleanup service start: %v", err)
		}
	})

	code, tty := deleteViaStartTTY(t, ctx, name)
	if code != 0 {
		t.Fatalf("DXAC21: delete %s (stopped container): exit %d; want 0; output: %s", name, code, tty.RawOutput())
	}

	if secretExists(ctx, t, "sag-res-"+name+".conn") {
		t.Fatalf("DXAC21: conn secret still exists after delete")
	}
}
