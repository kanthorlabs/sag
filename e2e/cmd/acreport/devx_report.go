package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/kanthorlabs/sag/e2e/devx"
)

const (
	statusOtherPhase    = "OTHER_PHASE"
	devxRequiredSubtest = "DX_Harness_Skeleton"
	devxSchema          = 1
	devxMaxLayer        = 10
	devxMainSuite       = "Main"
	devxReportDir       = "devx"
)

type suiteRun struct {
	Suite  string
	Commit string
	Run    testRun
}

type devxTestResult struct {
	Name   string `json:"name"`
	Layer  int    `json:"layer"`
	Status string `json:"status"`
}

type devxACResult struct {
	ID     string           `json:"id"`
	Run    string           `json:"run"`
	RunBy  string           `json:"run_by"`
	Status string           `json:"status"`
	Tests  []devxTestResult `json:"tests"`
}

type devxEnv struct {
	Date          string `json:"date"`
	OSVersion     string `json:"os_version"`
	PodmanVersion string `json:"podman_version"`
}

type devxGate struct {
	Layer     int      `json:"layer"`
	Phase     string   `json:"phase"`
	Passed    bool     `json:"passed"`
	Failing   []string `json:"failing"`
	RunFilter string   `json:"run_filter,omitempty"`
}

type devxReport struct {
	Schema      int            `json:"schema"`
	GeneratedAt string         `json:"generated_at"`
	Commit      string         `json:"commit"`
	Layer       int            `json:"layer"`
	Phase       string         `json:"phase"`
	Env         devxEnv        `json:"env"`
	ACs         []devxACResult `json:"acs"`
	Gate        devxGate       `json:"gate"`
}

func runDevx(args []string, stderr io.Writer, probe func() buildInfo) int {
	fs := flag.NewFlagSet("acreport devx", flag.ContinueOnError)
	fs.SetOutput(stderr)
	layer := fs.Int("layer", 0, "gate layer, 1 to 10")
	phase := fs.String("phase", "A", "build phase A or B")
	out := fs.String("out", "", "output directory for ac-report.json and ac-report.md")
	runFilter := fs.String("run", "", "test name pattern; select only matching tests in phase at layer<=N")
	var inputs inputPaths
	fs.Var(&inputs, "in", "go test -json events file; repeat for each suite")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if err := checkDevxArgs(*layer, *phase, *out, fs.NArg()); err != nil {
		fmt.Fprintf(stderr, "acreport: %v\n", err)
		return 2
	}
	runs, err := readDevxRuns(inputs)
	if err != nil {
		fmt.Fprintf(stderr, "acreport: %v\n", err)
		return 2
	}
	tapPath := filepath.Join(*out, "devx-unit", "node.tap")
	if sr, ok := tapSuiteRun(tapPath); ok {
		runs = append(runs, sr)
	}
	info := probe()
	r := buildDevxReport(*layer, *phase, *runFilter, runs, devx.ACTable, info)
	outDir := filepath.Join(*out, devxReportDir)
	if err := writeDevxReport(outDir, r); err != nil {
		fmt.Fprintf(stderr, "acreport: %v\n", err)
		return 2
	}
	if !r.Gate.Passed {
		fmt.Fprintf(stderr, "acreport: devx gate layer %d failed:\n", r.Gate.Layer)
		for _, f := range r.Gate.Failing {
			fmt.Fprintf(stderr, "  %s\n", f)
		}
		return 1
	}
	return 0
}

func checkDevxArgs(layer int, phase, out string, extra int) error {
	if layer < 1 || layer > devxMaxLayer {
		return fmt.Errorf("-layer must be 1 to %d", devxMaxLayer)
	}
	if phase != "A" && phase != "B" {
		return fmt.Errorf("-phase must be A or B")
	}
	if out == "" {
		return fmt.Errorf("-out is required")
	}
	if extra > 0 {
		return fmt.Errorf("unexpected arguments")
	}
	return nil
}

func readDevxRuns(paths []string) ([]suiteRun, error) {
	var runs []suiteRun
	for _, path := range paths {
		suite := suiteFromPath(path)
		f, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		tr, err := parseEvents(f)
		f.Close()
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		commit := readCommitFile(filepath.Dir(path))
		if commit == "" {
			return nil, fmt.Errorf("%s: missing commit file", path)
		}
		runs = append(runs, suiteRun{Suite: suite, Commit: commit, Run: tr})
	}
	return runs, nil
}

func suiteFromPath(path string) string {
	dir := filepath.Base(filepath.Dir(path))
	name := strings.TrimPrefix(dir, "devx-")
	if name == "" || name == dir {
		return dir
	}
	return name
}

func readCommitFile(dir string) string {
	data, err := os.ReadFile(filepath.Join(dir, "commit"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func buildDevxReport(layer int, phase, runFilter string, runs []suiteRun, table []devx.ACRow, info buildInfo) devxReport {
	r := devxReport{
		Schema:      devxSchema,
		GeneratedAt: time.Now().UTC().Format("2006-01-02T15:04:05.000Z"),
		Commit:      info.Commit,
		Layer:       layer,
		Phase:       phase,
		Env: devxEnv{
			Date:          time.Now().UTC().Format("2006-01-02"),
			OSVersion:     info.OSVersion,
			PodmanVersion: info.PodmanVersion,
		},
		ACs: []devxACResult{},
		Gate: devxGate{
			Layer:     layer,
			Phase:     phase,
			Failing:   []string{},
			RunFilter: runFilter,
		},
	}
	if info.GeneratedAt != "" {
		r.GeneratedAt = info.GeneratedAt
	}

	if runFilter != "" {
		re, err := regexp.Compile(runFilter)
		if err != nil {
			r.Gate.Failing = append(r.Gate.Failing, "invalid run filter: "+err.Error())
			r.Gate.Passed = false
			return r
		}
		matched := 0
		for _, row := range table {
			ac := buildDevxACFiltered(row, runs, layer, phase, re)
			r.ACs = append(r.ACs, ac)
			if ac.Status == statusFail {
				for _, t := range ac.Tests {
					if t.Layer <= layer && t.Status == statusFail {
						matched++
						r.Gate.Failing = append(r.Gate.Failing, t.Name)
					}
				}
			} else if ac.Status == statusPass {
				for _, t := range ac.Tests {
					if t.Layer <= layer && t.Status == statusPass && re.MatchString(t.Name) {
						matched++
					}
				}
			}
		}
		if matched == 0 {
			r.Gate.Failing = append(r.Gate.Failing, "no tests matched run filter: "+runFilter)
		}
		r.Gate.Failing = slices.Compact(slices.Sorted(slices.Values(r.Gate.Failing)))
		if r.Gate.Failing == nil {
			r.Gate.Failing = []string{}
		}
		r.Gate.Passed = len(r.Gate.Failing) == 0
		return r
	}

	for _, row := range table {
		ac := buildDevxAC(row, runs, layer, phase)
		r.ACs = append(r.ACs, ac)
		if ac.Status == statusFail {
			for _, t := range ac.Tests {
				if t.Layer <= layer && t.Status != statusPass {
					r.Gate.Failing = append(r.Gate.Failing, t.Name)
				}
			}
		}
	}

	if err := checkSkeletonPass(runs); err != nil {
		r.Gate.Failing = append(r.Gate.Failing, devxRequiredSubtest)
	}
	if err := checkCommitConsistency(runs); err != nil {
		r.Gate.Failing = append(r.Gate.Failing, "commit mismatch: "+err.Error())
	}

	r.Gate.Failing = slices.Compact(slices.Sorted(slices.Values(r.Gate.Failing)))
	if r.Gate.Failing == nil {
		r.Gate.Failing = []string{}
	}
	r.Gate.Passed = len(r.Gate.Failing) == 0
	return r
}

func buildDevxAC(row devx.ACRow, runs []suiteRun, layer int, phase string) devxACResult {
	ac := devxACResult{
		ID:    row.ID,
		Run:   row.Run,
		RunBy: row.RunBy,
		Tests: make([]devxTestResult, len(row.Tests)),
	}
	if !rowInPhase(row.Run, phase) {
		ac.Status = statusOtherPhase
		for i, t := range row.Tests {
			ac.Tests[i] = devxTestResult{Name: t.Name, Layer: t.Layer, Status: statusOtherPhase}
		}
		return ac
	}

	allNotImpl := true
	allLayerPass := true
	countInLayer := 0
	for i, t := range row.Tests {
		status := findDevxTestStatus(runs, t.Name)
		if t.Layer > layer {
			status = statusNotImplemented
		}
		ac.Tests[i] = devxTestResult{Name: t.Name, Layer: t.Layer, Status: status}
		if t.Layer <= layer {
			allNotImpl = false
			countInLayer++
			if status != statusPass {
				allLayerPass = false
			}
		}
	}

	switch {
	case allNotImpl || countInLayer == 0:
		ac.Status = statusNotImplemented
	case allLayerPass:
		ac.Status = statusPass
	default:
		ac.Status = statusFail
	}
	return ac
}

func buildDevxACFiltered(row devx.ACRow, runs []suiteRun, layer int, phase string, re *regexp.Regexp) devxACResult {
	ac := devxACResult{
		ID:    row.ID,
		Run:   row.Run,
		RunBy: row.RunBy,
		Tests: make([]devxTestResult, len(row.Tests)),
	}
	if !rowInPhase(row.Run, phase) {
		ac.Status = statusOtherPhase
		for i, t := range row.Tests {
			ac.Tests[i] = devxTestResult{Name: t.Name, Layer: t.Layer, Status: statusOtherPhase}
		}
		return ac
	}

	anyMatch := false
	allMatchPass := true
	anyMatchFail := false
	for i, t := range row.Tests {
		status := statusNotImplemented
		if t.Layer <= layer && re.MatchString(t.Name) {
			anyMatch = true
			status = findDevxTestStatus(runs, t.Name)
			if status != statusPass {
				allMatchPass = false
				anyMatchFail = true
			}
		}
		ac.Tests[i] = devxTestResult{Name: t.Name, Layer: t.Layer, Status: status}
	}

	switch {
	case !anyMatch:
		ac.Status = statusNotImplemented
	case anyMatchFail || !allMatchPass:
		ac.Status = statusFail
	default:
		ac.Status = statusPass
	}
	return ac
}

func rowInPhase(run, phase string) bool {
	switch run {
	case "AB":
		return true
	case "A":
		return phase == "A"
	case "B":
		return phase == "B"
	}
	return false
}

func findDevxTestStatus(runs []suiteRun, name string) string {
	for _, sr := range runs {
		for key, res := range sr.Run.Tests {
			parts := strings.Split(key.Test, "/")
			matched := (len(parts) >= 3 && parts[len(parts)-1] == name) ||
				(len(parts) == 1 && parts[0] == name)
			if matched {
				if res.Action == "pass" {
					return statusPass
				}
				return statusFail
			}
		}
	}
	return statusNotImplemented
}

const tapSagGuardTest = "DXAC44_SagGuard"

func tapSuiteRun(tapPath string) (suiteRun, bool) {
	f, err := os.Open(tapPath)
	if err != nil {
		return suiteRun{}, false
	}
	defer f.Close()
	res := parseTAP(f)
	action := "fail"
	if tapPassed(res) {
		action = "pass"
	}
	run := testRun{Tests: map[testKey]*testResult{}}
	syntheticKey := testKey{Package: "node", Test: "TestDevxUnit/harness/" + tapSagGuardTest}
	run.Tests[syntheticKey] = &testResult{Action: action}
	commit := readCommitFile(filepath.Dir(tapPath))
	return suiteRun{Suite: "unit", Commit: commit, Run: run}, true
}

func checkSkeletonPass(runs []suiteRun) error {
	hasMain := false
	for _, sr := range runs {
		if sr.Suite != devxMainSuite {
			continue
		}
		hasMain = true
		for key, res := range sr.Run.Tests {
			parts := strings.Split(key.Test, "/")
			if len(parts) >= 3 && parts[len(parts)-1] == devxRequiredSubtest {
				if res.Action == "pass" {
					return nil
				}
				return fmt.Errorf("%s failed", devxRequiredSubtest)
			}
		}
	}
	if !hasMain {
		return nil
	}
	return fmt.Errorf("%s not found in Main suite", devxRequiredSubtest)
}

func checkCommitConsistency(runs []suiteRun) error {
	var base string
	for _, sr := range runs {
		if base == "" {
			base = sr.Commit
			continue
		}
		if sr.Commit != base {
			return fmt.Errorf("suite %s commit %s != %s", sr.Suite, sr.Commit, base)
		}
	}
	return nil
}

func writeDevxReport(dir string, r devxReport) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, jsonFileName), append(data, '\n'), 0o644); err != nil {
		return err
	}
	var md bytes.Buffer
	if err := writeDevxMarkdown(&md, r); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, mdFileName), md.Bytes(), 0o644)
}

func writeDevxMarkdown(w io.Writer, r devxReport) error {
	var b strings.Builder
	gateResult := statusPass
	if !r.Gate.Passed {
		gateResult = statusFail + ": " + strings.Join(r.Gate.Failing, ", ")
	}
	fmt.Fprintf(&b, "# DX AC report\n\n")
	fmt.Fprintf(&b, "- Commit: `%s`\n", r.Commit)
	fmt.Fprintf(&b, "- Generated: %s\n", r.GeneratedAt)
	fmt.Fprintf(&b, "- Phase: %s\n", r.Phase)
	fmt.Fprintf(&b, "- Env: OS `%s`, Podman `%s`\n", r.Env.OSVersion, r.Env.PodmanVersion)
	fmt.Fprintf(&b, "- Gate layer %d phase %s: %s\n", r.Gate.Layer, r.Gate.Phase, cell(gateResult))
	if r.Gate.RunFilter != "" {
		selected := statusPass
		if !r.Gate.Passed {
			selected = statusFail
		}
		fmt.Fprintf(&b, "- Run filter: %s\n", r.Gate.RunFilter)
		fmt.Fprintf(&b, "- Selected: %s\n", selected)
	} else {
		skeletonResult := statusPass
		if slices.Contains(r.Gate.Failing, devxRequiredSubtest) {
			skeletonResult = statusFail
		}
		fmt.Fprintf(&b, "- Harness skeleton: %s\n", skeletonResult)
	}
	fmt.Fprintf(&b, "\n")

	if len(r.ACs) > 0 {
		writeRow(&b, []string{"AC", "Run", "Status"})
		writeRow(&b, []string{"---", "---", "---"})
		for _, ac := range r.ACs {
			writeRow(&b, []string{ac.ID, ac.Run, ac.Status})
		}
	}
	_, err := io.WriteString(w, b.String())
	return err
}
