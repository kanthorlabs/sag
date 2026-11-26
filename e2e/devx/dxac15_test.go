//go:build devxe2e

package devx_test

import (
	"context"
	"fmt"
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

	r := dxh.NewRunner()
	port, err := canaryPGPort(ctx, r)
	if err != nil {
		t.Fatalf("DXAC15: get canary PG port: %v", err)
	}

	canary := dxh.ActiveCanaryParts
	dsn := []byte(fmt.Sprintf("postgresql://%s:%s@host.containers.internal:%s/%s",
		canary.User, canary.Password, port, canary.Database))

	code, combined := runSAGWithInput(t, ctx, dsn,
		"credential", "create", "mydb-15",
		"--kind", "postgres",
		"--agent", "pi",
		"--stdin",
		"--tls", "disable",
	)
	if code != 0 {
		t.Fatalf("DXAC15: first create: exit %d; want 0; output: %s", code, combined)
	}

	code2, combined2 := runSAGWithInput(t, ctx, dsn,
		"credential", "create", "mydb-15",
		"--kind", "postgres",
		"--agent", "pi",
		"--stdin",
		"--tls", "disable",
	)
	if code2 != 1 {
		t.Fatalf("DXAC15: second create (duplicate): exit %d; want 1; output: %s", code2, combined2)
	}
}
