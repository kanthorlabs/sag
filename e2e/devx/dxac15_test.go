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
	credentialTests["DXAC15_CreateSecretExists"] = func(t *testing.T, ctx context.Context) {
		DXAC15_CreateSecretExists(t, ctx)
	}
}

func DXAC15_CreateSecretExists(t *testing.T, ctx context.Context) {
	t.Helper()
	dxh.EnsureCanary(t)

	uid := fmt.Sprintf("%d", os.Getuid())
	if err := dxh.WaitLaunchAgentIdle(ctx, dxh.NewRunner(), uid); err != nil {
		t.Fatalf("DXAC15: wait launch agent idle: %v", err)
	}

	r := dxh.NewRunner()
	port, err := canaryPGPort(ctx, r)
	if err != nil {
		t.Fatalf("DXAC15: get canary PG port: %v", err)
	}

	canary := dxh.ActiveCanaryParts
	dsn := (&url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(canary.User, canary.Password),
		Host:   "host.containers.internal:" + port,
		Path:   "/" + canary.Database,
	}).String()

	const name = "mydb-15"
	connSecretName := "sag-res-" + name + ".conn"

	cmd := exec.CommandContext(ctx, "podman", "secret", "create",
		"--label", "sag.devx-e2e=1",
		connSecretName, "-")
	cmd.Stdin = bytes.NewReader([]byte("placeholder"))
	if out, err2 := cmd.CombinedOutput(); err2 != nil {
		t.Fatalf("DXAC15: pre-create secret: %v\n%s", err2, out)
	}
	t.Cleanup(func() {
		r2 := dxh.NewRunner()
		r2.Output(context.Background(), "podman", "secret", "rm", connSecretName)
	})

	idBefore := ac15SecretField(ctx, t, connSecretName, "{{.ID}}")
	updBefore := ac15SecretField(ctx, t, connSecretName, "{{.UpdatedAt}}")

	code, combined := runSAGWithInput(t, ctx, []byte(dsn),
		"credential", "create", name,
		"--kind", "postgres",
		"--agent", "pi",
		"--tls", "disable",
		"--stdin",
	)
	if code != 1 {
		t.Fatalf("DXAC15: create with existing secret: exit %d; want 1; output: %s", code, combined)
	}

	wantMsg := fmt.Sprintf("secret %s exists; run sag credential delete %s", connSecretName, name)
	if !strings.Contains(combined, wantMsg) {
		t.Fatalf("DXAC15: output does not contain %q; output: %s", wantMsg, combined)
	}

	idAfter := ac15SecretField(ctx, t, connSecretName, "{{.ID}}")
	updAfter := ac15SecretField(ctx, t, connSecretName, "{{.UpdatedAt}}")

	if idBefore != idAfter {
		t.Fatalf("DXAC15: secret ID changed: %q → %q", idBefore, idAfter)
	}
	if updBefore != updAfter {
		t.Fatalf("DXAC15: secret UpdatedAt changed: %q → %q", updBefore, updAfter)
	}
}

func ac15SecretField(ctx context.Context, t *testing.T, name, format string) string {
	t.Helper()
	r := dxh.NewRunner()
	out, err := r.Output(ctx, "podman", "secret", "inspect", name, "--format", format)
	if err != nil {
		t.Fatalf("secret inspect %s %s: %v", name, format, err)
	}
	return strings.TrimSpace(string(out))
}
