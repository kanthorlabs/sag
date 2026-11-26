package dxh_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/kanthorlabs/sag/e2e/devx/dxh"
)

const testRunToken = "dxh-unit-test-token"

func holdRunLock(t *testing.T, devDir string) {
	t.Helper()
	lock := filepath.Join(devDir, "claims", "devx-e2e.lock")
	if err := os.MkdirAll(lock, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(lock, "owner"), []byte(fmt.Sprintf("%d %s", os.Getpid(), testRunToken)), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DEVX_RUN_TOKEN", testRunToken)
}

func TestSuiteEntryCreatesAndRemovesMarker(t *testing.T) {
	dir := t.TempDir()
	holdRunLock(t, dir)
	markerFile := filepath.Join(dir, "sub", "devx-run-active")

	r := goodMachineRunner()
	pcfg := dxh.PreconditionConfig{}
	tcfg := dxh.TeardownConfig{}

	cleanup := dxh.SuiteEntry(t, r, pcfg, tcfg, markerFile)

	if _, err := os.Stat(markerFile); err != nil {
		t.Fatalf("marker not created: %v", err)
	}

	cleanup()

	if _, err := os.Stat(markerFile); err == nil {
		t.Fatal("marker not removed after cleanup")
	}
}

func TestSuiteEntryWithExistingMarkerPreconditionsPass(t *testing.T) {
	dir := t.TempDir()
	holdRunLock(t, dir)
	markerFile := filepath.Join(dir, "local", "devx-run-active")
	if err := os.MkdirAll(filepath.Dir(markerFile), 0o700); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(markerFile, nil, 0o600); err != nil {
		t.Fatalf("write marker: %v", err)
	}

	r := goodMachineRunner()
	pcfg := dxh.PreconditionConfig{}
	tcfg := dxh.TeardownConfig{}

	cleanup := dxh.SuiteEntry(t, r, pcfg, tcfg, markerFile)
	defer cleanup()

	if _, err := os.Stat(markerFile); err != nil {
		t.Fatalf("marker not present after SuiteEntry: %v", err)
	}
}

func TestRunGroupCallsTeardownBeforeAndAfterSubtests(t *testing.T) {
	r, tcfg := newCleanTeardownFake("/home/u", "1000", "")
	ctx := context.Background()

	subtestRan := false
	dxh.RunGroup(t, ctx, r, tcfg, "test-group", nil, func(t *testing.T) {
		subtestRan = true
	})

	if !subtestRan {
		t.Fatal("subtests did not run")
	}

	teardownCalls := 0
	for _, call := range r.calls {
		if len(call) == 4 && call[0] == "podman" && call[1] == "rm" && call[2] == "-f" && call[3] == "sag" {
			teardownCalls++
		}
	}
	if teardownCalls != 2 {
		t.Fatalf("expected teardown called 2 times (before and after), got %d", teardownCalls)
	}
}

func TestRunGroupCallsSetupBeforeSubtests(t *testing.T) {
	r, tcfg := newCleanTeardownFake("/home/u", "1000", "")
	r.set("podman tag localhost/sag:devx-e2e docker.io/kanthorlabs/sag:main", nil, nil)
	ctx := context.Background()

	order := []string{}
	setup := func() error {
		order = append(order, "setup")
		return nil
	}
	subtests := func(t *testing.T) {
		order = append(order, "subtests")
	}

	dxh.RunGroup(t, ctx, r, tcfg, "test-group", setup, subtests)

	if len(order) != 2 || order[0] != "setup" || order[1] != "subtests" {
		t.Fatalf("wrong order: %v", order)
	}
}
