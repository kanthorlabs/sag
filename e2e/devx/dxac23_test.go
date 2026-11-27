//go:build devxe2e

package devx_test

import (
	"bytes"
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/kanthorlabs/sag/e2e/devx/dxh"
)

func init() {
	deleteTests["DXAC23_DeleteForeignSecret"] = func(t *testing.T, ctx context.Context) {
		DXAC23_DeleteForeignSecret(t, ctx)
	}
}

func DXAC23_DeleteForeignSecret(t *testing.T, ctx context.Context) {
	t.Helper()
	dxh.EnsureCanary(t)

	uid := fmt.Sprintf("%d", os.Getuid())
	r := dxh.NewRunner()
	if err := dxh.WaitLaunchAgentIdle(ctx, r, uid); err != nil {
		t.Fatalf("DXAC23: wait launch agent idle: %v", err)
	}

	port, err := canaryPGPort(ctx, r)
	if err != nil {
		t.Fatalf("DXAC23: canary PG port: %v", err)
	}

	canary := dxh.ActiveCanaryParts
	dsn := (&url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(canary.User, canary.Password),
		Host:   "host.containers.internal:" + port,
		Path:   "/" + canary.Database,
	}).String()

	const name = "dxac23-pg"
	connSecret := "sag-res-" + name + ".conn"

	code, _ := runSAGWithInput(t, ctx, []byte(dsn),
		"credential", "create", name,
		"--kind", "postgres",
		"--agent", "pi",
		"--tls", "disable",
		"--stdin",
	)
	if code != 0 {
		t.Fatalf("DXAC23: create %s: exit %d", name, code)
	}

	if err := dxh.WaitLaunchAgentIdle(ctx, r, uid); err != nil {
		t.Fatalf("DXAC23: wait launch agent idle after create: %v", err)
	}

	if _, err := r.Output(ctx, "podman", "secret", "rm", connSecret); err != nil {
		t.Fatalf("DXAC23: remove old conn secret: %v", err)
	}

	createSecCmd := exec.CommandContext(ctx, "podman", "secret", "create",
		"--driver=file",
		"--label=sag.devx-e2e=1",
		connSecret, "-",
	)
	createSecCmd.Stdin = bytes.NewBufferString("foreign-value")
	if out, err := createSecCmd.CombinedOutput(); err != nil {
		t.Fatalf("DXAC23: create foreign conn secret: %v\n%s", err, out)
	}

	t.Cleanup(func() {
		r.Output(ctx, "podman", "secret", "rm", connSecret)
	})

	var outBuf strings.Builder
	bin, err := dxh.TestBinary()
	if err != nil {
		t.Fatalf("DXAC23: TestBinary: %v", err)
	}
	groupDir := t.TempDir()
	binDir := t.TempDir()
	tty, startErr := dxh.StartTTY(ctx, groupDir, binDir, bin, "credential", "delete", name)
	if startErr != nil {
		t.Fatalf("DXAC23: StartTTY: %v", startErr)
	}
	if err := tty.Expect(ctx, "type the resource name"); err != nil {
		t.Fatalf("DXAC23: expect prompt: %v\noutput: %s", err, tty.RawOutput())
	}
	if err := tty.Send(name + "\n"); err != nil {
		t.Fatalf("DXAC23: send name: %v", err)
	}
	code, waitErr := tty.Wait(ctx)
	if waitErr != nil {
		t.Fatalf("DXAC23: wait: %v", waitErr)
	}
	outBuf.WriteString(tty.RawOutput())

	if code != 0 {
		t.Fatalf("DXAC23: delete %s: exit %d; want 0; output: %s", name, code, outBuf.String())
	}

	wantSkip := "skip " + connSecret + " not created by sag; not removed"
	foundSkip := false
	for _, line := range strings.Split(outBuf.String(), "\n") {
		if strings.TrimRight(line, "\r") == wantSkip {
			foundSkip = true
			break
		}
	}
	if !foundSkip {
		t.Fatalf("DXAC23: exact skip line %q not found; output: %s", wantSkip, outBuf.String())
	}

	if !secretExists(ctx, t, connSecret) {
		t.Fatalf("DXAC23: foreign conn secret was deleted; want it to be skipped")
	}
}
