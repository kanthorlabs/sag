package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
)

type testKey struct {
	Package string
	Test    string
}

type testResult struct {
	Action  string
	Elapsed float64
	Output  []string
}

func (r *testResult) failed() bool {
	return r.Action != "pass" && r.Action != "skip"
}

type testRun struct {
	Tests          map[testKey]*testResult
	FailedPackages []string
}

type goTestEvent struct {
	Action  string
	Package string
	Test    string
	Output  string
	Elapsed float64
}

func parseEvents(r io.Reader) (testRun, error) {
	run := testRun{Tests: map[testKey]*testResult{}}
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 16<<20)
	events := 0
	for scanner.Scan() {
		var ev goTestEvent
		if json.Unmarshal(scanner.Bytes(), &ev) != nil || ev.Action == "" {
			continue
		}
		events++
		if ev.Test == "" {
			if ev.Action == "fail" {
				run.FailedPackages = append(run.FailedPackages, ev.Package)
			}
			continue
		}
		key := testKey{Package: ev.Package, Test: ev.Test}
		res := run.Tests[key]
		if res == nil {
			res = &testResult{}
			run.Tests[key] = res
		}
		switch ev.Action {
		case "output":
			res.Output = append(res.Output, ev.Output)
		case "pass", "fail", "skip":
			res.Action = ev.Action
			res.Elapsed = ev.Elapsed
		}
	}
	if err := scanner.Err(); err != nil {
		return testRun{}, err
	}
	if events == 0 {
		return testRun{}, errors.New("no go test events")
	}
	return run, nil
}
