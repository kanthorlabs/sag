package main

import (
	"fmt"
	"maps"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/kanthorlabs/sag/e2e"
)

const (
	statusPass           = "PASS"
	statusFail           = "FAIL"
	statusNotImplemented = "NOT_IMPLEMENTED"
	statusNotRun         = "NOT_RUN"
	requiredSubtest      = "Harness_Skeleton"

	topLevelTest   = "TestE2E"
	maxMessageLen  = 500
	reportSchema   = 1
	noResultReason = "no result (timeout or crash)"
)

var phaseGroups = []string{"parallel", "sequential"}

type buildInfo struct {
	GeneratedAt  string
	Commit       string
	Dirty        bool
	Image        string
	ImageVersion string
}

type majorRun struct {
	Major int
	Run   testRun
}

type acResult struct {
	ID         string            `json:"id"`
	Layer      int               `json:"layer"`
	Test       string            `json:"test"`
	Status     string            `json:"status"`
	DurationMS int64             `json:"duration_ms"`
	ByMajor    map[string]string `json:"by_major"`
	Message    string            `json:"message"`
}

type otherResult struct {
	Test    string            `json:"test"`
	Status  string            `json:"status"`
	ByMajor map[string]string `json:"by_major"`
}

type gate struct {
	Layer   int      `json:"layer"`
	Passed  bool     `json:"passed"`
	Failing []string `json:"failing"`
}

type report struct {
	Schema       int           `json:"schema"`
	GeneratedAt  string        `json:"generated_at"`
	Commit       string        `json:"commit"`
	Dirty        bool          `json:"dirty"`
	Layer        int           `json:"layer"`
	Image        string        `json:"image"`
	ImageVersion string        `json:"image_version"`
	PGMajors     []int         `json:"pg_majors"`
	ACs          []acResult    `json:"acs"`
	Others       []otherResult `json:"others"`
	Gate         gate          `json:"gate"`
}

func buildReport(layer int, runs []majorRun, table []e2e.ACRow, info buildInfo) report {
	runs = slices.SortedFunc(slices.Values(runs), func(a, b majorRun) int { return a.Major - b.Major })
	r := report{
		Schema:       reportSchema,
		GeneratedAt:  info.GeneratedAt,
		Commit:       info.Commit,
		Dirty:        info.Dirty,
		Layer:        layer,
		Image:        info.Image,
		ImageVersion: info.ImageVersion,
		PGMajors:     []int{},
		ACs:          []acResult{},
		Others:       []otherResult{},
		Gate:         gate{Layer: layer, Failing: []string{}},
	}
	for _, mr := range runs {
		r.PGMajors = append(r.PGMajors, mr.Major)
	}
	for _, row := range table {
		ac := mergeMajors(row, runs)
		r.ACs = append(r.ACs, ac)
		if ac.Layer <= layer && ac.Status != statusPass {
			r.Gate.Failing = append(r.Gate.Failing, ac.Test)
		}
	}
	acNames := map[string]bool{}
	for _, row := range table {
		acNames[row.Test] = true
	}
	r.Others = otherSubtests(runs, acNames)
	others := map[string]bool{}
	for _, mr := range runs {
		for _, name := range otherFailures(mr.Run, acNames) {
			others[name] = true
		}
		if _, _, ok := findAC(mr.Run, requiredSubtest); !ok {
			others[requiredSubtest] = true
		}
	}
	r.Gate.Failing = append(r.Gate.Failing, slices.Sorted(maps.Keys(others))...)
	r.Gate.Passed = len(r.Gate.Failing) == 0
	return r
}

func mergeMajors(row e2e.ACRow, runs []majorRun) acResult {
	ac := acResult{ID: row.ID, Layer: row.Layer, Test: row.Test, ByMajor: map[string]string{}}
	ran, passed := 0, 0
	var messages []string
	for _, mr := range runs {
		major := strconv.Itoa(mr.Major)
		key, res, ok := findAC(mr.Run, row.Test)
		if !ok {
			ac.ByMajor[major] = statusNotImplemented
			messages = append(messages, fmt.Sprintf("pg%d: not run", mr.Major))
			continue
		}
		ran++
		if ac.Test == row.Test {
			ac.Test = key.Test
		}
		ac.DurationMS = max(ac.DurationMS, int64(math.Round(maxElapsed(mr.Run, key)*1000)))
		if res.Action == "pass" {
			ac.ByMajor[major] = statusPass
			passed++
			continue
		}
		ac.ByMajor[major] = statusFail
		messages = append(messages, fmt.Sprintf("pg%d: %s", mr.Major, failureMessage(res)))
	}
	switch {
	case ran == 0:
		ac.Status = statusNotImplemented
	case passed == len(runs):
		ac.Status = statusPass
	default:
		ac.Status = statusFail
		ac.Message = truncate(strings.Join(messages, "; "))
	}
	return ac
}

func maxElapsed(run testRun, parent testKey) float64 {
	elapsed := run.Tests[parent].Elapsed
	for key, res := range run.Tests {
		if key.Package == parent.Package && strings.HasPrefix(key.Test, parent.Test+"/") {
			elapsed = max(elapsed, res.Elapsed)
		}
	}
	return elapsed
}

func otherSubtests(runs []majorRun, acNames map[string]bool) []otherResult {
	results := map[string]*otherResult{}
	for _, mr := range runs {
		for key, res := range mr.Run.Tests {
			parts := strings.Split(key.Test, "/")
			if len(parts) != 3 || parts[0] != topLevelTest || !slices.Contains(phaseGroups, parts[1]) || acNames[parts[2]] {
				continue
			}
			o := results[key.Test]
			if o == nil {
				o = &otherResult{Test: key.Test, ByMajor: map[string]string{}}
				results[key.Test] = o
			}
			o.ByMajor[strconv.Itoa(mr.Major)] = statusFail
			if res.Action == "pass" {
				o.ByMajor[strconv.Itoa(mr.Major)] = statusPass
			}
		}
	}
	others := []otherResult{}
	for _, name := range slices.Sorted(maps.Keys(results)) {
		o := results[name]
		o.Status = statusPass
		for _, mr := range runs {
			major := strconv.Itoa(mr.Major)
			if _, ok := o.ByMajor[major]; !ok {
				o.ByMajor[major] = statusNotRun
			}
			if o.ByMajor[major] != statusPass {
				o.Status = statusFail
			}
		}
		others = append(others, *o)
	}
	return others
}

func findAC(run testRun, name string) (testKey, *testResult, bool) {
	keys := slices.SortedFunc(maps.Keys(run.Tests), func(a, b testKey) int {
		return strings.Compare(a.Package+" "+a.Test, b.Package+" "+b.Test)
	})
	for _, key := range keys {
		for _, group := range phaseGroups {
			if key.Test == topLevelTest+"/"+group+"/"+name {
				return key, run.Tests[key], true
			}
		}
	}
	return testKey{}, nil, false
}

func failureMessage(res *testResult) string {
	if res.Action == "" {
		return noResultReason
	}
	var lines []string
	for _, out := range res.Output {
		line := strings.TrimSpace(out)
		if line == "" || strings.HasPrefix(line, "=== ") || strings.HasPrefix(line, "--- ") {
			continue
		}
		lines = append(lines, line)
	}
	if len(lines) == 0 {
		return res.Action
	}
	return strings.Join(lines, " ")
}

func truncate(s string) string {
	runes := []rune(s)
	if len(runes) <= maxMessageLen {
		return s
	}
	return string(runes[:maxMessageLen]) + "..."
}

func underAC(test string, acNames map[string]bool) bool {
	parts := strings.Split(test, "/")
	return len(parts) >= 3 && parts[0] == topLevelTest && slices.Contains(phaseGroups, parts[1]) && acNames[parts[2]]
}

func otherFailures(run testRun, acNames map[string]bool) []string {
	var failures []string
	packagesWithFailedTests := map[string]bool{}
	for key, res := range run.Tests {
		if !res.failed() {
			continue
		}
		packagesWithFailedTests[key.Package] = true
		if underAC(key.Test, acNames) || hasFailedDescendant(run, key) {
			continue
		}
		failures = append(failures, key.Test)
	}
	for _, pkg := range run.FailedPackages {
		if !packagesWithFailedTests[pkg] {
			failures = append(failures, "package "+pkg)
		}
	}
	return failures
}

func hasFailedDescendant(run testRun, parent testKey) bool {
	for key, res := range run.Tests {
		if key.Package == parent.Package && strings.HasPrefix(key.Test, parent.Test+"/") && res.failed() {
			return true
		}
	}
	return false
}
