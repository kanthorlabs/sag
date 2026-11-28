package main

import (
	"bytes"
	"os"
	"slices"
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
	r := buildDevxReport(1, "A", "", []suiteRun{main}, devx.ACTable, buildInfo{})

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
	r := buildDevxReport(1, "A", "", []suiteRun{main}, devx.ACTable, buildInfo{})

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
	r := buildDevxReport(1, "A", "", []suiteRun{main}, devx.ACTable, buildInfo{Commit: "abc123"})
	if !r.Gate.Passed {
		t.Errorf("gate should pass with no layer-1 DX-ACs required: failing=%v", r.Gate.Failing)
	}

	main2 := devxParsed(t, "Main",
		devxSkeletonPass("Main"),
		devxFinished("Main", "DXAC01_Bootstrap", "fail"),
	)
	r2 := buildDevxReport(2, "A", "", []suiteRun{main2}, devx.ACTable, buildInfo{Commit: "abc123"})
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

	main3 := devxParsed(t, "Main",
		devxSkeletonPass("Main"),
	)
	r3 := buildDevxReport(2, "A", "", []suiteRun{main3}, devx.ACTable, buildInfo{Commit: "abc123"})
	for _, ac := range r3.ACs {
		if ac.Status == statusOtherPhase || ac.Status == statusNotImplemented {
			continue
		}
		for _, test := range ac.Tests {
			if test.Layer <= 2 && test.Status == statusNotImplemented {
				found := false
				for _, f := range r3.Gate.Failing {
					if f == test.Name {
						found = true
					}
				}
				if !found {
					t.Errorf("layer-2 NOT_IMPLEMENTED test %s not in gate.Failing: %v", test.Name, r3.Gate.Failing)
				}
			}
		}
	}
}

func TestDevxCommitMismatchFails(t *testing.T) {
	main := devxParsed(t, "Main", devxSkeletonPass("Main"))
	main.Commit = "abc123"
	pins := devxParsed(t, "Pins", devxSkeletonPass("Pins"))
	pins.Commit = "def456"

	r := buildDevxReport(1, "A", "", []suiteRun{main, pins}, devx.ACTable, buildInfo{Commit: "abc123"})
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
	r := buildDevxReport(1, "A", "", []suiteRun{main}, devx.ACTable, buildInfo{Commit: "abc123"})

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

func TestDevxRunFilterPass(t *testing.T) {
	main := devxParsed(t, "Main",
		devxFinished("Main", "DXAC01_Bootstrap", "pass"),
	)
	main.Commit = "abc"
	r := buildDevxReport(2, "A", "DXAC01_Bootstrap", []suiteRun{main}, devx.ACTable, buildInfo{Commit: "abc"})
	if !r.Gate.Passed {
		t.Errorf("run filter PASS: gate.Passed=false, failing=%v", r.Gate.Failing)
	}
	if r.Gate.RunFilter != "DXAC01_Bootstrap" {
		t.Errorf("RunFilter = %q, want DXAC01_Bootstrap", r.Gate.RunFilter)
	}
}

func TestDevxRunFilterNoMatchFails(t *testing.T) {
	main := devxParsed(t, "Main",
		devxFinished("Main", "DXAC01_Bootstrap", "pass"),
	)
	main.Commit = "abc"
	r := buildDevxReport(2, "A", "NOMATCH_XYZ", []suiteRun{main}, devx.ACTable, buildInfo{Commit: "abc"})
	if r.Gate.Passed {
		t.Error("run filter with no match: gate should fail")
	}
	found := false
	for _, f := range r.Gate.Failing {
		if strings.Contains(f, "no tests matched") {
			found = true
		}
	}
	if !found {
		t.Errorf("gate failing = %v, want 'no tests matched' message", r.Gate.Failing)
	}
}

func TestDevxRunFilterAbsentTestFails(t *testing.T) {
	main := devxParsed(t, "Main",
		devxFinished("Main", "DXAC02_ContainerAndCert", "pass"),
	)
	main.Commit = "abc"
	r := buildDevxReport(2, "A", "DXAC01_Bootstrap", []suiteRun{main}, devx.ACTable, buildInfo{Commit: "abc"})
	if r.Gate.Passed {
		t.Error("run filter with absent test: gate should fail")
	}
	found := false
	for _, f := range r.Gate.Failing {
		if strings.Contains(f, "DXAC01") {
			found = true
		}
	}
	if !found {
		t.Errorf("gate failing = %v, want DXAC01 in it", r.Gate.Failing)
	}
}

func TestDevxReadsEachSuite(t *testing.T) {
	dir := t.TempDir()

	for _, suite := range []string{"Main", "Pins"} {
		suiteDir := dir + "/devx-" + suite
		if err := os.MkdirAll(suiteDir, 0o755); err != nil {
			t.Fatal(err)
		}
		evs := flattenEvents(devxSkeletonPass(suite))
		data := encode(t, evs)
		if err := os.WriteFile(suiteDir+"/events.json", []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(suiteDir+"/commit", []byte("abc123\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	runs, err := readDevxRuns([]string{
		dir + "/devx-Main/events.json",
		dir + "/devx-Pins/events.json",
	})
	if err != nil {
		t.Fatalf("readDevxRuns: %v", err)
	}
	if len(runs) != 2 {
		t.Fatalf("got %d runs, want 2", len(runs))
	}
	suites := map[string]bool{}
	for _, sr := range runs {
		suites[sr.Suite] = true
		if sr.Commit != "abc123" {
			t.Errorf("suite %s commit = %q, want abc123", sr.Suite, sr.Commit)
		}
	}
	if !suites["Main"] || !suites["Pins"] {
		t.Errorf("suites = %v, want Main and Pins", suites)
	}
}

func TestDevxTestPhaseA(t *testing.T) {
	findAC := func(r devxReport) devxACResult {
		for _, ac := range r.ACs {
			if ac.ID == "DX-AC39" {
				return ac
			}
		}
		t.Fatal("DX-AC39 missing from report")
		return devxACResult{}
	}

	main := devxParsed(t, "Main",
		devxSkeletonPass("Main"),
		devxFinished("Main", "DXAC39_ClaudeCodeSetup", "pass"),
	)
	rB := buildDevxReport(9, "B", "", []suiteRun{main}, devx.ACTable, buildInfo{Commit: "abc123"})
	if ac := findAC(rB); ac.Status != statusPass {
		t.Errorf("phase B DX-AC39 status = %s, want %s; tests=%v", ac.Status, statusPass, ac.Tests)
	}
	if slices.Contains(rB.Gate.Failing, "DXAC39_DefaultPath") {
		t.Errorf("phase B gate failing = %v; DXAC39_DefaultPath runs in phase A only", rB.Gate.Failing)
	}

	rA := buildDevxReport(9, "A", "", []suiteRun{main}, devx.ACTable, buildInfo{Commit: "abc123"})
	if ac := findAC(rA); ac.Status != statusFail {
		t.Errorf("phase A DX-AC39 status = %s, want %s without DXAC39_DefaultPath", ac.Status, statusFail)
	}
	if !slices.Contains(rA.Gate.Failing, "DXAC39_DefaultPath") {
		t.Errorf("phase A gate failing = %v, want DXAC39_DefaultPath", rA.Gate.Failing)
	}
}
