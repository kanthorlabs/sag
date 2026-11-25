package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"maps"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/kanthorlabs/sag/e2e"
)

const (
	mergeCommand       = "merge"
	statusExcluded     = "EXCLUDED"
	ciExcludedAC       = "AC31"
	ciAC               = "AC35"
	macOSSuccess       = "success"
	ciLayer            = 6
	ciExclusionMessage = "excluded in CI: no Claude Code login"
)

var ciMajors = []int{14, 15, 16, 17, 18, 19}

var macOSResults = []string{macOSSuccess, "failure", "cancelled", "skipped"}

type mergeInput struct {
	Reports    []report
	MacOSBuild string
}

func runMerge(args []string, stderr io.Writer) int {
	flags := flag.NewFlagSet("acreport merge", flag.ContinueOnError)
	flags.SetOutput(stderr)
	var inputs inputPaths
	flags.Var(&inputs, "in", "ac-report.json of one CI matrix job; repeat for each major")
	macOS := flags.String("macos-build", "", "result of the build-macos job: "+strings.Join(macOSResults, ", "))
	out := flags.String("out", "", "output directory for "+jsonFileName+" and "+mdFileName)
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if err := checkMergeArgs(inputs, *macOS, *out, flags.NArg()); err != nil {
		fmt.Fprintf(stderr, "acreport merge: %v\n", err)
		return 2
	}
	reports, err := readReports(inputs)
	if err != nil {
		fmt.Fprintf(stderr, "acreport merge: %v\n", err)
		return 2
	}
	r := mergeReports(mergeInput{Reports: reports, MacOSBuild: *macOS}, e2e.ACTable)
	r.GeneratedAt = time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	if err := writeReport(*out, r); err != nil {
		fmt.Fprintf(stderr, "acreport merge: %v\n", err)
		return 2
	}
	for _, ac := range r.ACs {
		if ac.ID == ciAC && ac.Status != statusPass {
			fmt.Fprintf(stderr, "acreport merge: %s %s: %s\n", ciAC, ac.Status, ac.Message)
			return 1
		}
	}
	return 0
}

func checkMergeArgs(inputs []string, macOS, out string, extra int) error {
	switch {
	case len(inputs) == 0:
		return errors.New("-in is required")
	case !slices.Contains(macOSResults, macOS):
		return fmt.Errorf("-macos-build must be one of %s", strings.Join(macOSResults, ", "))
	case out == "":
		return errors.New("-out is required")
	case extra > 0:
		return errors.New("unexpected arguments")
	}
	return nil
}

func readReports(paths []string) ([]report, error) {
	seen := map[int]bool{}
	var reports []report
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		var r report
		if err := json.Unmarshal(data, &r); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		switch {
		case r.Schema != reportSchema:
			return nil, fmt.Errorf("%s: schema %d, want %d", path, r.Schema, reportSchema)
		case r.Layer != ciLayer:
			return nil, fmt.Errorf("%s: layer %d, want %d", path, r.Layer, ciLayer)
		case len(r.PGMajors) != 1:
			return nil, fmt.Errorf("%s: pg_majors %v, want exactly one major", path, r.PGMajors)
		case seen[r.PGMajors[0]]:
			return nil, fmt.Errorf("%s: major %d given more than one time", path, r.PGMajors[0])
		case len(reports) > 0 && r.Commit != reports[0].Commit:
			return nil, fmt.Errorf("%s: commit %q, want %q", path, r.Commit, reports[0].Commit)
		}
		seen[r.PGMajors[0]] = true
		reports = append(reports, r)
	}
	return reports, nil
}

func mergeReports(in mergeInput, table []e2e.ACRow) report {
	reports := slices.SortedFunc(slices.Values(in.Reports), func(a, b report) int { return a.PGMajors[0] - b.PGMajors[0] })
	r := report{
		Schema:   reportSchema,
		Layer:    ciLayer,
		PGMajors: []int{},
		ACs:      []acResult{},
		Others:   mergeOthers(reports),
		Gate:     gate{Layer: ciLayer, Failing: []string{}},
	}
	for _, in := range reports {
		r.PGMajors = append(r.PGMajors, in.PGMajors[0])
		r.Commit = in.Commit
		r.Dirty = r.Dirty || in.Dirty
	}
	for _, row := range table {
		var ac acResult
		switch row.ID {
		case ciExcludedAC:
			ac = mergeAC(row, reports)
			ac.Status = statusExcluded
			ac.Message = ciExclusionMessage
		case ciAC:
			ac = acResult{ID: row.ID, Layer: row.Layer, Test: row.Test, ByMajor: map[string]string{}, Status: statusPass}
			if failed := ac35Failures(reports, in.MacOSBuild, table); len(failed) > 0 {
				ac.Status = statusFail
				ac.Message = truncate(strings.Join(failed, "; "))
			}
		default:
			ac = mergeAC(row, reports)
		}
		r.ACs = append(r.ACs, ac)
		if ac.Status != statusPass && ac.Status != statusExcluded {
			r.Gate.Failing = append(r.Gate.Failing, ac.Test)
		}
	}
	for _, o := range r.Others {
		if o.Status != statusPass {
			r.Gate.Failing = append(r.Gate.Failing, o.Test)
		}
	}
	r.Gate.Passed = len(r.Gate.Failing) == 0
	return r
}

func mergeAC(row e2e.ACRow, reports []report) acResult {
	ac := acResult{ID: row.ID, Layer: row.Layer, Test: row.Test, Status: statusPass, ByMajor: map[string]string{}}
	var messages []string
	for i, in := range reports {
		major := strconv.Itoa(in.PGMajors[0])
		got, ok := findByID(in, row.ID)
		if !ok {
			ac.ByMajor[major] = statusNotRun
			ac.Status = statusFail
			messages = append(messages, fmt.Sprintf("pg%s: missing from report", major))
			continue
		}
		if i == 0 {
			ac.Test = got.Test
		}
		ac.ByMajor[major] = got.ByMajor[major]
		ac.DurationMS = max(ac.DurationMS, got.DurationMS)
		if got.ByMajor[major] != statusPass {
			ac.Status = statusFail
		}
		if got.Message != "" {
			messages = append(messages, got.Message)
		}
	}
	if ac.Status != statusPass {
		ac.Message = truncate(strings.Join(messages, "; "))
	}
	return ac
}

func mergeOthers(reports []report) []otherResult {
	results := map[string]*otherResult{}
	for _, in := range reports {
		for _, o := range in.Others {
			if results[o.Test] == nil {
				results[o.Test] = &otherResult{Test: o.Test, ByMajor: map[string]string{}}
			}
			major := strconv.Itoa(in.PGMajors[0])
			results[o.Test].ByMajor[major] = o.ByMajor[major]
		}
	}
	others := []otherResult{}
	for _, name := range slices.Sorted(maps.Keys(results)) {
		o := results[name]
		o.Status = statusPass
		for _, in := range reports {
			major := strconv.Itoa(in.PGMajors[0])
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

func ac35Failures(reports []report, macOS string, table []e2e.ACRow) []string {
	var failed []string
	var majors []int
	for _, in := range reports {
		majors = append(majors, in.PGMajors[0])
	}
	if missing := missingMajors(ciMajors, majors); len(missing) > 0 {
		failed = append(failed, "missing majors "+joinInts(missing))
	}
	if extra := missingMajors(majors, ciMajors); len(extra) > 0 {
		failed = append(failed, "unexpected majors "+joinInts(extra))
	}
	allowed := map[string]bool{}
	for _, row := range table {
		if row.ID == ciExcludedAC || row.ID == ciAC {
			allowed[row.Test] = true
		}
	}
	for _, in := range reports {
		var items []string
		for _, ac := range in.ACs {
			if ac.ID != ciExcludedAC && ac.ID != ciAC && ac.Status != statusPass {
				items = append(items, ac.ID+" "+ac.Status)
			}
		}
		for _, o := range in.Others {
			if o.Status != statusPass {
				items = append(items, o.Test+" "+o.Status)
			}
		}
		for _, test := range in.Gate.Failing {
			if !allowed[test[strings.LastIndex(test, "/")+1:]] {
				items = append(items, "gate "+test)
			}
		}
		if len(items) > 0 {
			failed = append(failed, fmt.Sprintf("pg%d: %s", in.PGMajors[0], strings.Join(items, ", ")))
		}
	}
	if macOS != macOSSuccess {
		failed = append(failed, fmt.Sprintf("build-macos %s, want %s", macOS, macOSSuccess))
	}
	return failed
}

func findByID(r report, id string) (acResult, bool) {
	for _, ac := range r.ACs {
		if ac.ID == id {
			return ac, true
		}
	}
	return acResult{}, false
}

func missingMajors(want, got []int) []int {
	var missing []int
	for _, m := range want {
		if !slices.Contains(got, m) {
			missing = append(missing, m)
		}
	}
	return missing
}

func joinInts(values []int) string {
	parts := make([]string, len(values))
	for i, v := range values {
		parts[i] = strconv.Itoa(v)
	}
	return strings.Join(parts, " ")
}
