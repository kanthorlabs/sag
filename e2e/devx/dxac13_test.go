//go:build devxe2e

package devx_test

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/kanthorlabs/sag/e2e/devx/dxh"
)

const loopbackWarn = "warn dsn host is a loopback address; in the container it is the container; for a database on this machine use host.containers.internal; on Linux the database must listen on a non-loopback address"

func init() {
	credentialTests["DXAC13_LoopbackWarn"] = DXAC13_LoopbackWarn
}

func DXAC13_LoopbackWarn(t *testing.T, ctx context.Context) {
	t.Helper()
	dxh.EnsureBootstrapped(t)
	dxh.EnsureCanary(t)

	uid := fmt.Sprintf("%d", os.Getuid())
	if err := dxh.WaitLaunchAgentIdle(ctx, dxh.NewRunner(), uid); err != nil {
		t.Fatalf("DXAC13: wait launch agent idle: %v", err)
	}

	dsn13 := "postgres://testuser:testpass@127.0.0.1:5432/testdb"
	name := "dxac13loop"

	_, combined := runSAGWithInput(t, ctx, []byte(dsn13),
		"credential", "create", name,
		"--kind", "postgres",
		"--agent", "claude-code",
		"--tls", "disable",
		"--stdin",
	)

	if !strings.Contains(combined, loopbackWarn) {
		t.Fatalf("DXAC13: output does not contain loopback warn; output: %s", combined)
	}

	for _, line := range strings.Split(combined, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "warn ") && strings.Contains(line, "loopback") {
			if line != loopbackWarn {
				t.Errorf("DXAC13: loopback warn line = %q; want %q", line, loopbackWarn)
			}
			break
		}
	}
}
