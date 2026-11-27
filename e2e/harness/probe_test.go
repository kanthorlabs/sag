package harness

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFilterStatsByPrefix(t *testing.T) {
	raw := `[
		{"name":"sag-e2e-ab12-sag","cpu_percent":"1.00%","pids":5},
		{"name":"other-container","cpu_percent":"0.00%","pids":1},
		{"name":"sag-e2e-ab12-pg17","cpu_percent":"0.50%","pids":3}
	]`
	got := filterStatsByPrefix(raw, "sag-e2e-ab12")
	if strings.Contains(got, "other-container") {
		t.Errorf("filtered output still contains other-container: %s", got)
	}
	for _, want := range []string{"sag-e2e-ab12-sag", "sag-e2e-ab12-pg17"} {
		if !strings.Contains(got, want) {
			t.Errorf("filtered output missing %q: %s", want, got)
		}
	}
}

func TestFilterStatsByPrefixInvalidJSON(t *testing.T) {
	raw := "not json"
	got := filterStatsByPrefix(raw, "prefix")
	if got != raw {
		t.Errorf("invalid JSON: got %q, want raw passthrough %q", got, raw)
	}
}

func TestFilterStatsByPrefixEmpty(t *testing.T) {
	raw := `[]`
	got := filterStatsByPrefix(raw, "sag-e2e-ab12")
	if got != "null" && got != "[]" {
		t.Errorf("empty array: got %q", got)
	}
}

func TestFormatBlock(t *testing.T) {
	ts := time.Date(2026, 11, 27, 10, 30, 0, 0, time.UTC)
	block := formatBlock(ts, "sag-e2e-ab12-pg17", "10.89.0.5",
		"sag-e2e-ab12-sag", "sag-e2e-ab12-sag-conformance",
		`[{"name":"sag-e2e-ab12-sag"}]`,
		"nslookup output\nnslookup_exit:0\nnc_exit:0\n",
		"nslookup output\nnslookup_exit:0\nnc_exit:0\n",
	)
	for _, want := range []string{
		"2026-11-27T10:30:00Z",
		"--- stats ---",
		"sag-e2e-ab12-sag",
		"probe:main",
		"probe:sibling",
		"pg=sag-e2e-ab12-pg17",
		"pgip=10.89.0.5",
		"nslookup_exit:0",
		"nc_exit:0",
	} {
		if !strings.Contains(block, want) {
			t.Errorf("block missing %q:\n%s", want, block)
		}
	}
}

func TestFormatBlockNoSibling(t *testing.T) {
	ts := time.Date(2026, 11, 27, 10, 30, 0, 0, time.UTC)
	block := formatBlock(ts, "sag-e2e-ab12-pg17", "10.89.0.5",
		"sag-e2e-ab12-sag", "",
		`[]`, "nslookup_exit:0\nnc_exit:0\n", "",
	)
	if strings.Contains(block, "probe:sibling") {
		t.Errorf("block with no sibling contains probe:sibling section:\n%s", block)
	}
}

func TestFilterEventLines(t *testing.T) {
	raw := strings.Join([]string{
		`{"Name":"sag-e2e-ab12-sag","Status":"start","Type":"container"}`,
		`{"Name":"other-container","Status":"start","Type":"container"}`,
		`{"Name":"sag-e2e-ab12-pg17","Status":"start","Type":"container"}`,
		`not-json`,
		``,
	}, "\n")
	got := string(filterEventLines(raw, "sag-e2e-ab12"))
	if strings.Contains(got, "other-container") {
		t.Errorf("filtered events contain other-container:\n%s", got)
	}
	for _, want := range []string{"sag-e2e-ab12-sag", "sag-e2e-ab12-pg17"} {
		if !strings.Contains(got, want) {
			t.Errorf("filtered events missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "not-json") {
		t.Errorf("filtered events contain unparseable line:\n%s", got)
	}
}

func TestWriteProbeBlock(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "probe.log")
	ts := time.Date(2026, 11, 27, 10, 30, 0, 0, time.UTC)
	block := formatBlock(ts, "pg", "1.2.3.4", "main", "", "{}", "out\n", "")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.WriteString(block)
	_ = f.Close()
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "2026-11-27T10:30:00Z") {
		t.Errorf("probe.log missing timestamp: %s", got)
	}
}
