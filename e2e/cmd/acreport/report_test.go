package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/kanthorlabs/sag/e2e"
)

const pkg = "github.com/kanthorlabs/sag/e2e"

type event struct {
	Action  string
	Test    string
	Output  string
	Elapsed float64
}

func encode(t *testing.T, events []event) string {
	t.Helper()
	var b strings.Builder
	for _, ev := range events {
		line, err := json.Marshal(map[string]any{
			"Time":    "2026-11-25T12:00:00Z",
			"Action":  ev.Action,
			"Package": pkg,
			"Test":    ev.Test,
			"Output":  ev.Output,
			"Elapsed": ev.Elapsed,
		})
		if err != nil {
			t.Fatal(err)
		}
		b.Write(line)
		b.WriteByte('\n')
	}
	return b.String()
}

func finished(name, action string, elapsed float64, output ...string) []event {
	events := []event{{Action: "run", Test: name}}
	for _, line := range output {
		events = append(events, event{Action: "output", Test: name, Output: line})
	}
	return append(events, event{Action: action, Test: name, Elapsed: elapsed})
}

func parsed(t *testing.T, events ...[]event) testRun {
	t.Helper()
	run, err := parseEvents(strings.NewReader("not json\n" + encode(t, slices.Concat(events...))))
	if err != nil {
		t.Fatal(err)
	}
	return run
}

func find(t *testing.T, r report, id string) acResult {
	t.Helper()
	for _, ac := range r.ACs {
		if ac.ID == id {
			return ac
		}
	}
	t.Fatalf("AC %s missing from report", id)
	return acResult{}
}

func TestStatusRules(t *testing.T) {
	run := parsed(t,
		[]event{{Action: "run", Test: "TestE2E"}, {Action: "run", Test: "TestE2E/parallel"}},
		finished("TestE2E/parallel/AC01_ConfigCheckRejects", "pass", 1.5),
		finished("TestE2E/parallel/AC02_StartRefusesBadSecrets", "fail", 2, "=== RUN   TestE2E/parallel/AC02_StartRefusesBadSecrets\n", "    ac02_test.go:10: exit 0, want 1\n", "--- FAIL: TestE2E/parallel/AC02_StartRefusesBadSecrets (2.00s)\n"),
		finished("TestE2E/parallel/AC03_JWTRejects", "skip", 0.1, "    ac03_test.go:5: no prerequisite\n"),
		[]event{{Action: "run", Test: "TestE2E/sequential/AC04_JWTGenerateRefuses"}},
	)
	r := buildReport(1, []majorRun{{Major: 17, Run: run}}, e2e.ACTable, buildInfo{})

	cases := []struct {
		id, status, test string
		durationMS       int64
		message          string
	}{
		{"AC01", statusPass, "TestE2E/parallel/AC01_ConfigCheckRejects", 1500, ""},
		{"AC02", statusFail, "TestE2E/parallel/AC02_StartRefusesBadSecrets", 2000, "exit 0, want 1"},
		{"AC03", statusFail, "TestE2E/parallel/AC03_JWTRejects", 100, "no prerequisite"},
		{"AC04", statusFail, "TestE2E/sequential/AC04_JWTGenerateRefuses", 0, "no result"},
	}
	for _, c := range cases {
		got := find(t, r, c.id)
		if got.Status != c.status || got.Test != c.test || got.DurationMS != c.durationMS {
			t.Errorf("%s = %+v, want status %s test %s duration %d", c.id, got, c.status, c.test, c.durationMS)
		}
		if !strings.Contains(got.Message, c.message) {
			t.Errorf("%s message = %q, want it to contain %q", c.id, got.Message, c.message)
		}
		if got.ByMajor["17"] != c.status {
			t.Errorf("%s by_major = %v, want 17: %s", c.id, got.ByMajor, c.status)
		}
	}
}

func TestNotImplementedWhenAbsent(t *testing.T) {
	run := parsed(t, finished("TestE2E/parallel/AC01_ConfigCheckRejects", "pass", 1))
	r := buildReport(6, []majorRun{{Major: 17, Run: run}}, e2e.ACTable, buildInfo{})

	if len(r.ACs) != len(e2e.ACTable) {
		t.Fatalf("report has %d ACs, want %d", len(r.ACs), len(e2e.ACTable))
	}
	got := find(t, r, "AC31")
	if got.Status != statusNotImplemented || got.Test != "AC31_ClaudeCode" || got.ByMajor["17"] != statusNotImplemented {
		t.Errorf("AC31 = %+v, want NOT_IMPLEMENTED with test AC31_ClaudeCode", got)
	}
	if !slices.Contains(r.Gate.Failing, "AC31_ClaudeCode") {
		t.Errorf("gate failing = %v, want AC31_ClaudeCode", r.Gate.Failing)
	}
}

func TestGateFailsOnEarlierLayer(t *testing.T) {
	layer1Pass := slices.Concat(
		finished("TestE2E/parallel/AC01_ConfigCheckRejects", "pass", 1),
		finished("TestE2E/parallel/AC02_StartRefusesBadSecrets", "pass", 1),
		finished("TestE2E/parallel/AC03_JWTRejects", "pass", 1),
		finished("TestE2E/parallel/AC04_JWTGenerateRefuses", "pass", 1),
	)
	cases := []struct {
		name    string
		layer   int
		events  []event
		passed  bool
		failing []string
	}{
		{"all layer 1 pass", 1, layer1Pass, true, nil},
		{"later layer AC fails", 1, slices.Concat(layer1Pass,
			[]event{{Action: "run", Test: "TestE2E"}, {Action: "run", Test: "TestE2E/sequential"}},
			finished("TestE2E/sequential/AC08_RollbackOnly/begin", "fail", 1),
			finished("TestE2E/sequential/AC08_RollbackOnly", "fail", 1),
			[]event{{Action: "fail", Test: "TestE2E/sequential"}, {Action: "fail", Test: "TestE2E"}, {Action: "fail"}},
		), true, nil},
		{"earlier layer AC fails", 2, layer1Pass, false, []string{"AC05_C2BlocksWrites", "AC06_SingleStatement", "AC07_NoPassThrough", "AC08_RollbackOnly", "AC09_RefusesPG13", "AC10_TLSVerifyFull", "AC11_ExtensionApproval", "AC12_OperatorCollision"}},
		{"same layer AC fails", 1, slices.Concat(layer1Pass[:6], finished("TestE2E/parallel/AC04_JWTGenerateRefuses", "fail", 1)), false, []string{"TestE2E/parallel/AC04_JWTGenerateRefuses"}},
		{"other subtest fails", 1, slices.Concat(layer1Pass,
			[]event{{Action: "run", Test: "TestE2E"}, {Action: "run", Test: "TestE2E/parallel"}},
			finished("TestE2E/parallel/Harness_Skeleton", "fail", 1),
			[]event{{Action: "fail", Test: "TestE2E/parallel"}, {Action: "fail", Test: "TestE2E"}, {Action: "fail"}},
		), false, []string{"TestE2E/parallel/Harness_Skeleton"}},
		{"suite setup fails", 1, slices.Concat(finished("TestE2E", "fail", 1), []event{{Action: "fail"}}), false, []string{"AC01_ConfigCheckRejects", "AC02_StartRefusesBadSecrets", "AC03_JWTRejects", "AC04_JWTGenerateRefuses", "TestE2E"}},
		{"build fails", 1, []event{{Action: "fail"}}, false, []string{"AC01_ConfigCheckRejects", "AC02_StartRefusesBadSecrets", "AC03_JWTRejects", "AC04_JWTGenerateRefuses", "package " + pkg}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := buildReport(c.layer, []majorRun{{Major: 17, Run: parsed(t, c.events)}}, e2e.ACTable, buildInfo{})
			if r.Gate.Passed != c.passed || r.Gate.Layer != c.layer || !slices.Equal(r.Gate.Failing, c.failing) {
				t.Errorf("gate = %+v, want passed %v failing %v", r.Gate, c.passed, c.failing)
			}
		})
	}
}

func TestMarkdownHas35Rows(t *testing.T) {
	run := parsed(t, finished("TestE2E/parallel/AC01_ConfigCheckRejects", "fail", 1, "    x.go:1: bad | pipe\n"))
	r := buildReport(1, []majorRun{{Major: 14, Run: run}, {Major: 17, Run: run}}, e2e.ACTable, buildInfo{Commit: "abc123", Dirty: true, Image: "sha256:feed"})

	var b bytes.Buffer
	if err := writeMarkdown(&b, r); err != nil {
		t.Fatal(err)
	}
	md := b.String()
	for _, want := range []string{"abc123", "sha256:feed", "| AC | Layer | Test | Status | PG14 | PG17 | Duration | Message |", "FAIL", `bad \| pipe`} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown misses %q:\n%s", want, md)
		}
	}
	rows := 0
	for line := range strings.SplitSeq(md, "\n") {
		if strings.HasPrefix(line, "| AC") && !strings.HasPrefix(line, "| AC |") {
			rows++
		}
	}
	if rows != 35 {
		t.Errorf("markdown has %d AC rows, want 35", rows)
	}
}

func TestMergeMajors(t *testing.T) {
	pass := func(elapsed float64) testRun {
		return parsed(t, finished("TestE2E/parallel/AC01_ConfigCheckRejects", "pass", elapsed), finished("TestE2E/parallel/AC03_JWTRejects", "pass", elapsed))
	}
	failAC01 := parsed(t, finished("TestE2E/parallel/AC01_ConfigCheckRejects", "fail", 1, "    a.go:1: boom\n"))
	r := buildReport(1, []majorRun{{Major: 19, Run: failAC01}, {Major: 14, Run: pass(1)}, {Major: 17, Run: pass(3)}}, e2e.ACTable, buildInfo{})

	if !slices.Equal(r.PGMajors, []int{14, 17, 19}) {
		t.Errorf("pg_majors = %v, want [14 17 19]", r.PGMajors)
	}
	ac01 := find(t, r, "AC01")
	wantByMajor := map[string]string{"14": statusPass, "17": statusPass, "19": statusFail}
	if ac01.Status != statusFail || fmt.Sprint(ac01.ByMajor) != fmt.Sprint(wantByMajor) || ac01.DurationMS != 3000 || !strings.Contains(ac01.Message, "pg19: a.go:1: boom") {
		t.Errorf("AC01 = %+v, want FAIL by_major %v duration 3000 message with pg19: a.go:1: boom", ac01, wantByMajor)
	}
	ac03 := find(t, r, "AC03")
	if ac03.Status != statusFail || ac03.ByMajor["19"] != statusNotImplemented || !strings.Contains(ac03.Message, "pg19: not run") {
		t.Errorf("AC03 = %+v, want FAIL with pg19 NOT_IMPLEMENTED", ac03)
	}
	allPass := buildReport(1, []majorRun{{Major: 14, Run: pass(1)}, {Major: 17, Run: pass(2)}}, e2e.ACTable, buildInfo{})
	if got := find(t, allPass, "AC01"); got.Status != statusPass || got.DurationMS != 2000 {
		t.Errorf("AC01 = %+v, want PASS duration 2000", got)
	}
	if got := find(t, allPass, "AC02"); got.Status != statusNotImplemented {
		t.Errorf("AC02 = %+v, want NOT_IMPLEMENTED", got)
	}
}

func writeEvents(t *testing.T, dir string, major int, events []event) string {
	t.Helper()
	path := filepath.Join(dir, fmt.Sprintf("pg%d", major), "events.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(encode(t, events)), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestRunExitCodes(t *testing.T) {
	layer1 := slices.Concat(
		finished("TestE2E/parallel/AC01_ConfigCheckRejects", "pass", 1),
		finished("TestE2E/parallel/AC02_StartRefusesBadSecrets", "pass", 1),
		finished("TestE2E/parallel/AC03_JWTRejects", "pass", 1),
		finished("TestE2E/sequential/AC04_JWTGenerateRefuses", "pass", 1),
	)
	info := buildInfo{GeneratedAt: "2026-11-25T12:00:00.000Z", Commit: "abc", Image: "sha256:feed"}
	probe := func() buildInfo { return info }

	dir := t.TempDir()
	in14 := writeEvents(t, dir, 14, layer1)
	in17 := writeEvents(t, dir, 17, layer1)
	out := filepath.Join(dir, "out")
	var stderr bytes.Buffer
	if code := run([]string{"-layer", "1", "-in", in14, "-in", in17, "-out", out}, &stderr, probe); code != 0 {
		t.Fatalf("exit = %d, want 0; stderr: %s", code, stderr.String())
	}
	raw, err := os.ReadFile(filepath.Join(out, "ac-report.json"))
	if err != nil {
		t.Fatal(err)
	}
	var got report
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if got.Schema != 1 || got.Layer != 1 || got.Commit != "abc" || got.Image != "sha256:feed" || !slices.Equal(got.PGMajors, []int{14, 17}) || len(got.ACs) != 35 || !got.Gate.Passed {
		t.Errorf("report = %+v", got)
	}
	if _, err := os.Stat(filepath.Join(out, "ac-report.md")); err != nil {
		t.Error(err)
	}

	stderr.Reset()
	in19 := writeEvents(t, dir, 19, slices.Concat(layer1[:6], finished("TestE2E/sequential/AC04_JWTGenerateRefuses", "fail", 1)))
	if code := run([]string{"-layer", "1", "-in", in14, "-in", in19, "-out", out}, &stderr, probe); code != 1 {
		t.Errorf("exit = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "TestE2E/sequential/AC04_JWTGenerateRefuses") {
		t.Errorf("stderr = %q, want the failing test", stderr.String())
	}

	bad := filepath.Join(dir, "events.json")
	if err := os.WriteFile(bad, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"-layer", "1", "-in", bad, "-out", out},
		{"-layer", "1", "-in", in14, "-in", in14, "-out", out},
		{"-layer", "0", "-in", in14, "-out", out},
		{"-layer", "1", "-out", out},
		{"-layer", "1", "-in", in14},
	} {
		if code := run(args, &stderr, probe); code != 2 {
			t.Errorf("run(%v) = %d, want 2", args, code)
		}
	}
}
