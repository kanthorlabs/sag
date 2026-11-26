package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/kanthorlabs/sag/e2e/devx"
)

func devxParsed(t *testing.T, suite string, events ...[]event) suiteRun {
	t.Helper()
	run, err := parseEvents(strings.NewReader(encode(t, flattenEvents(events...))))
	if err != nil {
		t.Fatal(err)
	}
	return suiteRun{Suite: suite, Run: run}
}

func flattenEvents(slices ...[]event) []event {
	var out []event
	for _, s := range slices {
		out = append(out, s...)
	}
	return out
}

func devxFinished(suite, name, action string) []event {
	top := "TestDevx" + suite
	return []event{
		{Action: "run", Test: top},
		{Action: "run", Test: top + "/harness"},
		{Action: "run", Test: top + "/harness/" + name},
		{Action: action, Test: top + "/harness/" + name, Elapsed: 0.1},
		{Action: "pass", Test: top + "/harness"},
		{Action: "pass", Test: top},
	}
}

func devxSkeletonPass(suite string) []event {
	return devxFinished(suite, devxRequiredSubtest, "pass")
}

func TestDevxOtherPhase(t *testing.T) {
	main := devxParsed(t, "Main",
		devxSkeletonPass("Main"),
	)
	r := buildDevxReport(1, "A", []suiteRun{main}, devx.ACTable, buildInfo{})

	for _, ac := range r.ACs {
		if ac.Run == "B" && ac.Status != statusOtherPhase {
			t.Errorf("AC %s run=%s phase A: status=%s, want %s", ac.ID, ac.Run, ac.Status, statusOtherPhase)
		}
	}
	for _, ac := range r.ACs {
		if ac.Run == "AB" && ac.Status == statusOtherPhase {
			t.Errorf("AC %s run=AB should not be OTHER_PHASE in phase A", ac.ID)
		}
	}
}

func TestDevxEnvBlockHasNoIP(t *testing.T) {
	main := devxParsed(t, "Main", devxSkeletonPass("Main"))
	r := buildDevxReport(1, "A", []suiteRun{main}, devx.ACTable, buildInfo{})

	var md bytes.Buffer
	if err := writeDevxMarkdown(&md, r); err != nil {
		t.Fatal(err)
	}
	text := md.String()
	for _, ip := range []string{"100.", "192.168.", "172.", "::1", "fd7a:"} {
		if strings.Contains(text, ip) {
			t.Errorf("devx markdown contains IP-like text %q", ip)
		}
	}
}

func TestDevxGateRule(t *testing.T) {
	main := devxParsed(t, "Main", devxSkeletonPass("Main"))
	r := buildDevxReport(1, "A", []suiteRun{main}, devx.ACTable, buildInfo{Commit: "abc123"})
	if !r.Gate.Passed {
		t.Errorf("gate should pass with no layer-1 DX-ACs required: failing=%v", r.Gate.Failing)
	}

	main2 := devxParsed(t, "Main",
		devxSkeletonPass("Main"),
		devxFinished("Main", "DXAC01_Bootstrap", "fail"),
	)
	r2 := buildDevxReport(2, "A", []suiteRun{main2}, devx.ACTable, buildInfo{Commit: "abc123"})
	if r2.Gate.Passed {
		t.Error("gate should fail when a layer-2 AC fails at layer 2")
	}
	found := false
	for _, f := range r2.Gate.Failing {
		if strings.Contains(f, "DXAC01") {
			found = true
		}
	}
	if !found {
		t.Errorf("gate failing = %v, want DXAC01 in it", r2.Gate.Failing)
	}
}

func TestDevxCommitMismatchFails(t *testing.T) {
	main := devxParsed(t, "Main", devxSkeletonPass("Main"))
	main.Commit = "abc123"
	pins := devxParsed(t, "Pins", devxSkeletonPass("Pins"))
	pins.Commit = "def456"

	r := buildDevxReport(1, "A", []suiteRun{main, pins}, devx.ACTable, buildInfo{Commit: "abc123"})
	if r.Gate.Passed {
		t.Error("gate should fail on commit mismatch between suites")
	}
	found := false
	for _, f := range r.Gate.Failing {
		if strings.Contains(f, "commit") {
			found = true
		}
	}
	if !found {
		t.Errorf("gate failing = %v, want commit mismatch in it", r.Gate.Failing)
	}
}

func TestDevxTestLayers(t *testing.T) {
	main := devxParsed(t, "Main", devxSkeletonPass("Main"))
	r := buildDevxReport(1, "A", []suiteRun{main}, devx.ACTable, buildInfo{Commit: "abc123"})

	for _, ac := range r.ACs {
		if ac.Status == statusOtherPhase {
			continue
		}
		for _, test := range ac.Tests {
			if test.Layer > 1 && test.Status != statusNotImplemented {
				t.Errorf("AC %s test %s layer %d: status=%s, want NOT_IMPLEMENTED at layer 1", ac.ID, test.Name, test.Layer, test.Status)
			}
		}
	}
}
