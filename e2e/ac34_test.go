//go:build e2e

package e2e

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/kanthorlabs/sag/e2e/harness"
)

func init() {
	harness.Register(harness.AC{
		ID:    "AC34",
		Phase: harness.PhaseParallel,
		Run:   runAC34,
	})
}

func runAC34(t *testing.T, env harness.Env) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	res, err := env.Exec(ctx, harness.ContainerSAG, "sag", "jwt", "generate", "a")
	if err != nil {
		t.Fatalf("podman exec: %v", err)
	}
	if res.ExitCode == 0 {
		t.Fatalf("sag jwt generate without a terminal exit = 0; stdout %q; want non-zero", res.Stdout)
	}
	if !strings.Contains(res.Stderr, "not a terminal") {
		t.Fatalf("sag jwt generate stderr = %q; want the no-terminal cause", res.Stderr)
	}
	if res.Stdout != "" {
		t.Errorf("sag jwt generate without a terminal stdout = %q; want empty", res.Stdout)
	}
}
