package main

import (
	"bufio"
	"io"
	"strings"
)

type tapResult struct {
	Total  int
	Failed int
}

func parseTAP(r io.Reader) tapResult {
	var res tapResult
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "ok ") {
			res.Total++
		} else if strings.HasPrefix(line, "not ok ") {
			res.Total++
			res.Failed++
		}
	}
	return res
}

func tapPassed(r tapResult) bool {
	return r.Total > 0 && r.Failed == 0
}
