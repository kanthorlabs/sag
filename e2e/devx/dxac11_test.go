//go:build devxe2e

package devx_test

import (
	"context"
	"testing"

	"github.com/kanthorlabs/sag/e2e/devx/dxh"
)

func init() {
	credentialTests["DXAC11_CreateRejectsForm"] = func(t *testing.T, ctx context.Context) {
		DXAC11_CreateRejectsForm(t, ctx)
	}
}

func DXAC11_CreateRejectsForm(t *testing.T, ctx context.Context) {
	t.Helper()
	dxh.EnsureCanary(t)

	fakeDSN := []byte("postgres://user:pass@db.example.com/mydb")
	code, combined := runSAGWithInput(t, ctx, fakeDSN,
		"credential", "create", "mydb-11",
		"--kind", "postgres",
		"--agent", "pi",
		"--tls", "disable",
	)
	if code != 3 {
		t.Fatalf("DXAC11: piped stdin without --stdin: exit %d; want 3; output: %s", code, combined)
	}
}
