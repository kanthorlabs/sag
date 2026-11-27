package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/kanthorlabs/sag/e2e"
)

func ciReport(major int) report {
	m := strconv.Itoa(major)
	r := report{
		Schema:   reportSchema,
		Commit:   "abc",
		Layer:    6,
		PGMajors: []int{major},
		Others:   []otherResult{{Test: "TestE2E/parallel/Harness_Skeleton", Status: statusPass, ByMajor: map[string]string{m: statusPass}}},
		Gate:     gate{Layer: 6, Passed: true, Failing: []string{}},
	}
	for _, row := range e2e.ACTable {
		ac := acResult{ID: row.ID, Layer: row.Layer, Test: "TestE2E/parallel/" + row.Test, Status: statusPass, DurationMS: int64(major), ByMajor: map[string]string{m: statusPass}}
		switch row.ID {
		case ciExcludedAC:
			ac.Test, ac.Status, ac.Message, ac.ByMajor[m] = row.Test, statusExcluded, skipMessage, statusExcluded
		case ciAC:
			ac.Test, ac.Status, ac.ByMajor[m] = row.Test, statusNotImplemented, statusNotImplemented
		}
		r.ACs = append(r.ACs, ac)
	}
	return r
}

func ciReports(majors ...int) []report {
	var reports []report
	for _, m := range majors {
		reports = append(reports, ciReport(m))
	}
	return reports
}

func setAC(r report, id, status, message string) {
	for i := range r.ACs {
		if r.ACs[i].ID == id {
			r.ACs[i].Status = status
			r.ACs[i].Message = message
			for k := range r.ACs[i].ByMajor {
				r.ACs[i].ByMajor[k] = status
			}
		}
	}
}

func TestAC35NeedsAllMajors(t *testing.T) {
	all := mergeReports(mergeInput{Reports: ciReports(19, 14, 15, 16, 17, 18), MacOSBuild: macOSSuccess}, e2e.ACTable)
	ac35 := find(t, all, ciAC)
	if ac35.Status != statusPass || ac35.Message != "" || len(ac35.ByMajor) != 0 {
		t.Errorf("AC35 = %+v, want PASS with no message and no by_major", ac35)
	}
	if !slices.Equal(all.PGMajors, ciMajors) || !all.Gate.Passed || len(all.Gate.Failing) != 0 || all.Commit != "abc" || all.Layer != 6 {
		t.Errorf("merged = %+v", all)
	}
	ac01 := find(t, all, "AC01")
	if ac01.Status != statusPass || len(ac01.ByMajor) != 6 || ac01.ByMajor["19"] != statusPass || ac01.DurationMS != 19 || ac01.Test != "TestE2E/parallel/AC01_ConfigCheckRejects" {
		t.Errorf("AC01 = %+v, want PASS for 6 majors with duration 19", ac01)
	}

	missing := mergeReports(mergeInput{Reports: ciReports(14, 15, 16, 17, 18), MacOSBuild: macOSSuccess}, e2e.ACTable)
	ac35 = find(t, missing, ciAC)
	if ac35.Status != statusFail || !strings.Contains(ac35.Message, "19") {
		t.Errorf("AC35 = %+v, want FAIL that names major 19", ac35)
	}
	if missing.Gate.Passed || !slices.Equal(missing.Gate.Failing, []string{"AC35_CI"}) {
		t.Errorf("gate = %+v, want failing [AC35_CI]", missing.Gate)
	}

	failing := ciReports(14, 15, 16, 17, 18, 19)
	setAC(failing[2], "AC10", statusFail, "pg16: boom")
	merged := mergeReports(mergeInput{Reports: failing, MacOSBuild: macOSSuccess}, e2e.ACTable)
	if got := find(t, merged, "AC10"); got.Status != statusFail || got.ByMajor["16"] != statusFail || got.ByMajor["14"] != statusPass || !strings.Contains(got.Message, "pg16: boom") {
		t.Errorf("AC10 = %+v, want FAIL on pg16", got)
	}
	if got := find(t, merged, ciAC); got.Status != statusFail || !strings.Contains(got.Message, "pg16") || !strings.Contains(got.Message, "AC10") {
		t.Errorf("AC35 = %+v, want FAIL that names pg16 and AC10", got)
	}
	if !slices.Equal(merged.Gate.Failing, []string{"TestE2E/parallel/AC10_TLSVerifyFull", "AC35_CI"}) {
		t.Errorf("gate failing = %v", merged.Gate.Failing)
	}

	other := ciReports(14, 15, 16, 17, 18, 19)
	other[0].Others[0].Status = statusFail
	other[0].Others[0].ByMajor["14"] = statusFail
	merged = mergeReports(mergeInput{Reports: other, MacOSBuild: macOSSuccess}, e2e.ACTable)
	if got := find(t, merged, ciAC); got.Status != statusFail || !strings.Contains(got.Message, "Harness_Skeleton") {
		t.Errorf("AC35 = %+v, want FAIL that names Harness_Skeleton", got)
	}
	if !slices.Contains(merged.Gate.Failing, "TestE2E/parallel/Harness_Skeleton") {
		t.Errorf("gate failing = %v, want Harness_Skeleton", merged.Gate.Failing)
	}

	pkgFail := ciReports(14, 15, 16, 17, 18, 19)
	pkgFail[5].Gate.Failing = []string{"package " + pkg}
	if got := find(t, mergeReports(mergeInput{Reports: pkgFail, MacOSBuild: macOSSuccess}, e2e.ACTable), ciAC); got.Status != statusFail || !strings.Contains(got.Message, "package "+pkg) {
		t.Errorf("AC35 = %+v, want FAIL that names the package failure", got)
	}
}

func TestAC35NeedsMacOSBuild(t *testing.T) {
	for _, result := range []string{"failure", "cancelled", "skipped"} {
		r := mergeReports(mergeInput{Reports: ciReports(14, 15, 16, 17, 18, 19), MacOSBuild: result}, e2e.ACTable)
		if got := find(t, r, ciAC); got.Status != statusFail || !strings.Contains(got.Message, "build-macos") || !strings.Contains(got.Message, result) {
			t.Errorf("macOS %s: AC35 = %+v, want FAIL that names build-macos %s", result, got, result)
		}
		if r.Gate.Passed {
			t.Errorf("macOS %s: gate passed, want failed", result)
		}
	}
}

func TestAC31ExcludedInCI(t *testing.T) {
	cases := map[string]func(r *report){
		"excluded": func(*report) {},
		"failed":   func(r *report) { setAC(*r, ciExcludedAC, statusFail, "no login") },
		"not run":  func(r *report) { setAC(*r, ciExcludedAC, statusNotImplemented, "") },
		"in gate": func(r *report) {
			setAC(*r, ciExcludedAC, statusFail, "no login")
			r.Gate.Passed = false
			r.Gate.Failing = []string{"TestE2E/parallel/AC31_ClaudeCode", "AC35_CI"}
		},
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			reports := ciReports(14, 15, 16, 17, 18, 19)
			for i := range reports {
				change(&reports[i])
			}
			r := mergeReports(mergeInput{Reports: reports, MacOSBuild: macOSSuccess}, e2e.ACTable)
			ac31 := find(t, r, ciExcludedAC)
			if ac31.Status != statusExcluded || ac31.Message != ciExclusionMessage || len(ac31.ByMajor) != 6 {
				t.Errorf("AC31 = %+v, want EXCLUDED with message %q and 6 majors", ac31, ciExclusionMessage)
			}
			if got := find(t, r, ciAC); got.Status != statusPass {
				t.Errorf("AC35 = %+v, want PASS", got)
			}
			if !r.Gate.Passed || len(r.Gate.Failing) != 0 {
				t.Errorf("gate = %+v, want passed", r.Gate)
			}
		})
	}
}

func TestAC35NeedsEachRow(t *testing.T) {
	removeAC := func(r *report, id string) {
		r.ACs = slices.DeleteFunc(r.ACs, func(ac acResult) bool { return ac.ID == id })
	}
	cases := map[string]struct {
		change func(r *report)
		want   string
	}{
		"empty acs and others": {func(r *report) { r.ACs, r.Others = nil, nil }, "AC01 missing"},
		"AC10 absent":          {func(r *report) { removeAC(r, "AC10") }, "AC10 missing"},
		"skeleton absent":      {func(r *report) { r.Others = nil }, requiredSubtest + " missing"},
		"skeleton other name": {func(r *report) {
			r.Others[0].Test = "TestE2E/parallel/Harness_Skeleton_Copy"
		}, requiredSubtest + " missing"},
		"PASS with FAIL by_major": {func(r *report) {
			for i := range r.ACs {
				if r.ACs[i].ID == "AC10" {
					r.ACs[i].ByMajor["16"] = statusFail
				}
			}
		}, "AC10 PASS (by_major FAIL)"},
		"AC05 EXCLUDED":        {func(r *report) { setAC(*r, "AC05", statusExcluded, skipMessage) }, "AC05 EXCLUDED"},
		"AC05 NOT_IMPLEMENTED": {func(r *report) { setAC(*r, "AC05", statusNotImplemented, "") }, "AC05 NOT_IMPLEMENTED"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			reports := ciReports(14, 15, 16, 17, 18, 19)
			c.change(&reports[2])
			r := mergeReports(mergeInput{Reports: reports, MacOSBuild: macOSSuccess}, e2e.ACTable)
			if got := find(t, r, ciAC); got.Status != statusFail || !strings.Contains(got.Message, "pg16: "+c.want) {
				t.Errorf("AC35 = %+v, want FAIL that names pg16: %s", got, c.want)
			}
			if r.Gate.Passed {
				t.Errorf("gate = %+v, want failed", r.Gate)
			}
		})
	}

	empty := ciReports(14, 15, 16, 17, 18, 19)
	for i := range empty {
		empty[i].ACs, empty[i].Others = nil, nil
	}
	if got := find(t, mergeReports(mergeInput{Reports: empty, MacOSBuild: macOSSuccess}, e2e.ACTable), ciAC); got.Status != statusFail {
		t.Errorf("AC35 for 6 empty inputs = %+v, want FAIL", got)
	}

	extra := mergeReports(mergeInput{Reports: ciReports(13, 14, 15, 16, 17, 18, 19), MacOSBuild: macOSSuccess}, e2e.ACTable)
	if got := find(t, extra, ciAC); got.Status != statusFail || !strings.Contains(got.Message, "unexpected majors 13") {
		t.Errorf("AC35 = %+v, want FAIL that names unexpected major 13", got)
	}
}

func TestMergeCarriesDirty(t *testing.T) {
	reports := ciReports(14, 15, 16, 17, 18, 19)
	if mergeReports(mergeInput{Reports: reports, MacOSBuild: macOSSuccess}, e2e.ACTable).Dirty {
		t.Error("dirty = true for clean inputs, want false")
	}
	reports[3].Dirty = true
	if !mergeReports(mergeInput{Reports: reports, MacOSBuild: macOSSuccess}, e2e.ACTable).Dirty {
		t.Error("dirty = false with one dirty input, want true")
	}
}

func writeReportFile(t *testing.T, dir string, r report) string {
	t.Helper()
	path := filepath.Join(dir, fmt.Sprintf("pg%d", r.PGMajors[0]), jsonFileName)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func mergeArgs(paths []string, macOS, out string) []string {
	args := []string{mergeCommand}
	for _, p := range paths {
		args = append(args, "-in", p)
	}
	return append(args, "-macos-build", macOS, "-out", out)
}

func TestRunMergeExitCodes(t *testing.T) {
	dir := t.TempDir()
	var paths []string
	for _, r := range ciReports(14, 15, 16, 17, 18, 19) {
		paths = append(paths, writeReportFile(t, dir, r))
	}
	out := filepath.Join(dir, "ac35")
	probe := func() buildInfo { return buildInfo{} }
	var stderr bytes.Buffer
	if code := run(mergeArgs(paths, macOSSuccess, out), &stderr, probe); code != 0 {
		t.Fatalf("exit = %d, want 0; stderr: %s", code, stderr.String())
	}
	raw, err := os.ReadFile(filepath.Join(out, jsonFileName))
	if err != nil {
		t.Fatal(err)
	}
	var got report
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if got.Schema != 1 || got.Commit != "abc" || got.GeneratedAt == "" || !slices.Equal(got.PGMajors, ciMajors) || find(t, got, ciAC).Status != statusPass {
		t.Errorf("merged report = %+v", got)
	}
	md, err := os.ReadFile(filepath.Join(out, mdFileName))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"| PG14 | PG15 | PG16 | PG17 | PG18 | PG19 |", "| AC35 | 6 | `AC35_CI` | PASS |", "| AC31 | 5 | `AC31_ClaudeCode` | EXCLUDED |"} {
		if !strings.Contains(string(md), want) {
			t.Errorf("markdown misses %q:\n%s", want, md)
		}
	}

	stderr.Reset()
	if code := run(mergeArgs(paths, "failure", out), &stderr, probe); code != 1 {
		t.Errorf("exit with macOS failure = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "build-macos") {
		t.Errorf("stderr = %q, want build-macos", stderr.String())
	}
	if code := run(mergeArgs(paths[:5], macOSSuccess, out), &stderr, probe); code != 1 {
		t.Errorf("exit with 5 majors = %d, want 1", code)
	}

	gateOnly := slices.Clone(paths)
	gateOnly[0] = writeReportFile(t, filepath.Join(dir, "extra"), func() report {
		r := ciReport(14)
		r.Others = append(r.Others, otherResult{Test: "TestE2E/parallel/Extra_Check", Status: statusPass, ByMajor: map[string]string{"14": statusPass}})
		return r
	}())
	stderr.Reset()
	if code := run(mergeArgs(gateOnly, macOSSuccess, out), &stderr, probe); code != 1 {
		t.Errorf("exit with a failed merged gate = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "TestE2E/parallel/Extra_Check") {
		t.Errorf("stderr = %q, want Extra_Check", stderr.String())
	}

	write := func(name string, change func(*report)) string {
		r := ciReport(14)
		change(&r)
		return writeReportFile(t, filepath.Join(dir, name), r)
	}
	twoMajors := write("two", func(r *report) { r.PGMajors = []int{14, 15} })
	wrongSchema := write("schema", func(r *report) { r.Schema = 2 })
	wrongLayer := write("layer", func(r *report) { r.Layer = 5 })
	otherCommit := write("commit", func(r *report) { r.Commit = "def" })
	garbage := filepath.Join(dir, "garbage.json")
	if err := os.WriteFile(garbage, []byte("not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	for name, args := range map[string][]string{
		"missing file":   mergeArgs([]string{filepath.Join(dir, "none", jsonFileName)}, macOSSuccess, out),
		"no input":       mergeArgs(nil, macOSSuccess, out),
		"no out":         mergeArgs(paths, macOSSuccess, "")[:len(mergeArgs(paths, macOSSuccess, ""))-2],
		"bad macos":      mergeArgs(paths, "passed", out),
		"no macos":       {mergeCommand, "-in", paths[0], "-out", out},
		"extra argument": append(mergeArgs(paths, macOSSuccess, out), "x"),
		"duplicate":      mergeArgs([]string{paths[0], paths[0]}, macOSSuccess, out),
		"two majors":     mergeArgs([]string{twoMajors}, macOSSuccess, out),
		"wrong schema":   mergeArgs([]string{wrongSchema}, macOSSuccess, out),
		"wrong layer":    mergeArgs([]string{wrongLayer}, macOSSuccess, out),
		"other commit":   mergeArgs([]string{paths[1], otherCommit}, macOSSuccess, out),
		"not json":       mergeArgs([]string{garbage}, macOSSuccess, out),
	} {
		stderr.Reset()
		if code := run(args, &stderr, probe); code != 2 {
			t.Errorf("%s: exit = %d, want 2; stderr: %s", name, code, stderr.String())
		}
	}
}
