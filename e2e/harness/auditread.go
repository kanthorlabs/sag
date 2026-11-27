package harness

import (
	"encoding/json"
	"errors"
	"strings"
	"time"
)

func auditDates(start, end time.Time) []string {
	day := time.Date(start.UTC().Year(), start.UTC().Month(), start.UTC().Day(), 0, 0, 0, 0, time.UTC)
	last := end.UTC()
	var dates []string
	for !day.After(last) {
		dates = append(dates, day.Format("2006-01-02"))
		day = day.AddDate(0, 0, 1)
	}
	return dates
}

func completeAuditLines(out string) ([]json.RawMessage, error) {
	if i := strings.LastIndexByte(out, '\n'); i >= 0 {
		out = out[:i+1]
	} else {
		out = ""
	}
	var records []json.RawMessage
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if !json.Valid([]byte(line)) {
			return nil, errors.New("a complete line is not JSON")
		}
		records = append(records, json.RawMessage(line))
	}
	return records, nil
}
