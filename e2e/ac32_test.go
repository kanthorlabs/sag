//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/kanthorlabs/sag/e2e/harness"
)

func init() {
	harness.Register(harness.AC{
		ID:    "AC32",
		Phase: harness.PhaseParallel,
		Run:   runAC32,
	})
}

type ac32Inspect struct {
	EffectiveCaps []string
	BoundingCaps  []string
	Config        struct {
		User string
	}
	HostConfig struct {
		ReadonlyRootfs bool
		SecurityOpt    []string
	}
	NetworkSettings struct {
		Ports map[string][]struct {
			HostIp   string
			HostPort string
		}
	}
}

func runAC32(t *testing.T, env harness.Env) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	res, err := env.Podman(ctx, "inspect", env.ContainerName(harness.ContainerSAG))
	if err != nil {
		t.Fatalf("podman inspect: %v", err)
	}
	if res.ExitCode != 0 {
		t.Fatalf("podman inspect exit = %d; stderr %q", res.ExitCode, res.Stderr)
	}
	var out []ac32Inspect
	if err := json.Unmarshal([]byte(res.Stdout), &out); err != nil {
		t.Fatalf("podman inspect output: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("podman inspect returned %d containers; want 1", len(out))
	}
	c := out[0]

	if !c.HostConfig.ReadonlyRootfs {
		t.Error("ReadonlyRootfs = false; want true")
	}
	wantUser := strconv.Itoa(harness.ContainerFlags.UID) + ":" + strconv.Itoa(harness.ContainerFlags.GID)
	if c.Config.User != wantUser {
		t.Errorf("Config.User = %q; want %q", c.Config.User, wantUser)
	}
	if len(c.EffectiveCaps) != 0 {
		t.Errorf("EffectiveCaps = %q; want none", c.EffectiveCaps)
	}
	if len(c.BoundingCaps) != 0 {
		t.Errorf("BoundingCaps = %q; want none", c.BoundingCaps)
	}
	if !slices.Contains(c.HostConfig.SecurityOpt, "no-new-privileges") {
		t.Errorf("SecurityOpt = %q; want no-new-privileges", c.HostConfig.SecurityOpt)
	}
	port := strconv.Itoa(harness.ContainerFlags.ContainerPort) + "/tcp"
	bindings := c.NetworkSettings.Ports[port]
	if len(bindings) == 0 {
		t.Fatalf("Ports[%s] has no binding", port)
	}
	for _, b := range bindings {
		if !harness.PublishHostAllowed(b.HostIp) {
			t.Errorf("Ports[%s] host IP %q; want 127.0.0.1 or an address in 100.64.0.0/10", port, b.HostIp)
		}
	}
}
