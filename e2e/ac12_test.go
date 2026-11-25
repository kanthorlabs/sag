//go:build e2e

package e2e

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kanthorlabs/sag/e2e/harness"
)

const (
	ac12Sub      = "ac12"
	ac12Resource = "citext"
	ac12Database = "sag_citext"
	ac12Check    = "check operator_names: pass: "
	ac12Operator = "~~*"
)

func init() {
	harness.Register(harness.AC{
		ID:     "AC12",
		Phase:  harness.PhaseParallel,
		Agents: []harness.AgentSpec{{Sub: ac12Sub, Name: "AC12"}},
		Resources: []harness.ResourceSpec{{
			Name:       ac12Resource,
			Containers: []string{harness.ContainerSAG},
			Database:   ac12Database,
			Agents:     []string{ac12Sub},
		}},
		Fixture: "CREATE EXTENSION IF NOT EXISTS citext",
		Run:     runAC12,
	})
}

func runAC12(t *testing.T, env harness.Env) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	res, err := env.Exec(ctx, harness.ContainerSAG, "sag", "resource", "test", ac12Resource)
	if err != nil {
		t.Fatalf("podman exec sag resource test %s: %v", ac12Resource, err)
	}
	if res.ExitCode != 0 {
		t.Fatalf("sag resource test %s exited %d; output %q", ac12Resource, res.ExitCode, res.Stdout+res.Stderr)
	}
	for _, line := range strings.Split(res.Stdout, "\n") {
		names, ok := strings.CutPrefix(strings.TrimSpace(line), ac12Check)
		if ok && slices.Contains(strings.Fields(names), ac12Operator) {
			return
		}
	}
	t.Fatalf("sag resource test %s output %q; want the citext operator %s in operator_names", ac12Resource, res.Stdout, ac12Operator)
}
