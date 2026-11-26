//go:build devxe2e

package devx_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/kanthorlabs/sag/e2e/devx/dxh"
)

func init() {
	credentialTests["DXAC14_CreateStdin"] = func(t *testing.T, ctx context.Context) {
		DXAC14_CreateStdin(t, ctx)
	}
}

func DXAC14_CreateStdin(t *testing.T, ctx context.Context) {
	t.Helper()
	dxh.EnsureCanary(t)

	r := dxh.NewRunner()
	port, err := canaryPGPort(ctx, r)
	if err != nil {
		t.Fatalf("DXAC14: get canary PG port: %v", err)
	}

	canary := dxh.ActiveCanaryParts
	dsn := fmt.Sprintf("postgresql://%s:%s@host.containers.internal:%s/%s",
		canary.User, canary.Password, port, canary.Database)

	code, combined := runSAGWithInput(t, ctx, []byte(dsn),
		"credential", "create", "mydb-14",
		"--kind", "postgres",
		"--agent", "pi",
		"--stdin",
		"--tls", "disable",
	)
	if code != 0 {
		t.Fatalf("DXAC14: create with stdin: exit %d; want 0; output: %s", code, combined)
	}

	listOut, err := runSAG(t, ctx, "credential", "list")
	if err != nil {
		t.Fatalf("DXAC14: credential list: %v", err)
	}
	if !strings.Contains(string(listOut), "mydb-14") {
		t.Fatalf("DXAC14: mydb-14 not in credential list:\n%s", listOut)
	}
}

func canaryPGPort(ctx context.Context, r dxh.Runner) (string, error) {
	out, err := r.Output(ctx, "podman", "port", "sag-canary-pg", "5432/tcp")
	if err != nil {
		return "", fmt.Errorf("podman port sag-canary-pg 5432/tcp: %w", err)
	}
	line := strings.TrimSpace(string(out))
	if i := strings.LastIndex(line, ":"); i >= 0 {
		return line[i+1:], nil
	}
	return "", fmt.Errorf("unexpected port output: %q", line)
}
