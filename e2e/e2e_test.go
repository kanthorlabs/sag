//go:build e2e

package e2e

import (
	"context"
	"os"
	"testing"

	"github.com/kanthorlabs/sag/e2e/harness"
)

func subtestName(id string) string {
	for _, row := range ACTable {
		if row.ID == id {
			return row.Test
		}
	}
	return id
}

func TestE2E(t *testing.T) {
	acs := harness.WithoutSkipped(harness.Registered(), os.Getenv(harness.SkipEnv))
	suite, err := harness.NewSuite(t.Context(), acs)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if t.Failed() {
			if err := suite.SaveLogs(context.Background(), "report"); err != nil {
				t.Logf("save logs: %v", err)
			}
		}
		if err := suite.Close(context.Background()); err != nil {
			t.Errorf("teardown: %v", err)
		}
	})

	stopProbe := suite.StartProbe("report")
	t.Run("parallel", func(t *testing.T) {
		for _, ac := range harness.InPhase(acs, harness.PhaseParallel) {
			t.Run(subtestName(ac.ID), func(t *testing.T) {
				t.Parallel()
				ac.Run(t, suite)
			})
		}
	})
	stopProbe()
	t.Run("sequential", func(t *testing.T) {
		for _, ac := range harness.InPhase(acs, harness.PhaseSequential) {
			t.Run(subtestName(ac.ID), func(t *testing.T) {
				ac.Run(t, suite)
			})
		}
	})
}
