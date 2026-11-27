//go:build devxe2e

package devx_test

import (
	"context"
	"strings"
	"testing"

	"github.com/kanthorlabs/sag/e2e/devx/dxh"
)

func init() {
	credentialTests["DXAC09_CreateCanary"] = func(t *testing.T, ctx context.Context) {
		DXAC09_CreateCanary(t, ctx)
	}
}

func DXAC09_CreateCanary(t *testing.T, ctx context.Context) {
	t.Helper()
	dxh.EnsureCanary(t)

	transcript := string(dxh.ActiveCanaryTranscript)
	canary := dxh.ActiveCanaryParts

	if !strings.Contains(transcript, dxh.CanaryPrompt) {
		t.Fatalf("DXAC09: transcript does not contain prompt %q", dxh.CanaryPrompt)
	}

	for _, part := range []string{canary.Password, canary.Host, canary.User, canary.Database} {
		if strings.Contains(transcript, part) {
			t.Fatalf("DXAC09: transcript contains canary part; transcript must not echo the value")
		}
	}

	if canary.Host != "host.containers.internal" {
		t.Fatalf("DXAC09: canary host = %q; want %q", canary.Host, "host.containers.internal")
	}

	const finalLine = "resource canary created; SAG restarted; run sag resource test canary"
	if !strings.Contains(transcript, finalLine) {
		t.Fatalf("DXAC09: transcript does not contain PRD 7.5 final line %q", finalLine)
	}

	r := dxh.NewRunner()
	testOut, testErr := r.Output(ctx, "podman", "exec", "sag", "sag", "resource", "test", "canary")
	if testErr != nil {
		t.Fatalf("DXAC09: sag resource test canary: %v\n%s", testErr, testOut)
	}
}
