package main

import (
	"fmt"
	"io"

	"github.com/kanthorlabs/sag/e2e"
)

const (
	mergeCommand   = "merge"
	statusExcluded = "EXCLUDED"
	ciExcludedAC   = "AC31"
	ciAC           = "AC35"
	macOSSuccess   = "success"
)

var ciMajors = []int{14, 15, 16, 17, 18, 19}

type mergeInput struct {
	Reports    []report
	MacOSBuild string
}

func runMerge(args []string, stderr io.Writer) int {
	fmt.Fprintln(stderr, "acreport: merge is not implemented")
	return 2
}

func mergeReports(in mergeInput, table []e2e.ACRow) report {
	return report{}
}
