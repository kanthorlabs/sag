package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func withStateDir(t *testing.T, dir string) {
	t.Helper()
	old := paths.State
	paths.State = dir
	t.Cleanup(func() { paths.State = old })
}

func TestPrintsLinesUnchanged(t *testing.T) {
	dir := t.TempDir()
	auditDir := filepath.Join(dir, "audit")
	if err := os.MkdirAll(auditDir, 0o700); err != nil {
		t.Fatalf("mkdir audit: %v", err)
	}

	lineShort := `{"ts":"2025-07-04T00:00:00.000Z","request_id":"01J000000000000000000000A1","sub":"test-agent"}` + "\n"
	longLine := `{"ts":"2025-07-04T00:00:00.001Z","request_id":"01J000000000000000000000A2","sql":"` + strings.Repeat("x", 65*1024) + `"}` + "\n"
	noNewline := `{"ts":"2025-07-04T00:00:00.002Z","request_id":"01J000000000000000000000A3"}`

	content := lineShort + longLine + noNewline
	auditFile := filepath.Join(auditDir, "audit-2025-07-04.jsonl")
	if err := os.WriteFile(auditFile, []byte(content), 0o600); err != nil {
		t.Fatalf("write audit file: %v", err)
	}

	withStateDir(t, dir)

	var out bytes.Buffer
	var errOut bytes.Buffer
	std := IO{Out: &out, Err: &errOut}
	code := auditRead([]string{"--date", "2025-07-04"}, std)
	if code != ExitOK {
		t.Fatalf("exit %d; want %d; stderr: %s", code, ExitOK, errOut.String())
	}
	if got := out.String(); got != content {
		t.Fatalf("output does not match input:\ngot  %d bytes\nwant %d bytes", len(got), len(content))
	}
}

func TestDateFormat(t *testing.T) {
	dir := t.TempDir()
	withStateDir(t, dir)

	cases := []struct {
		date string
	}{
		{"2025-13-01"},
		{"2025-00-01"},
		{"not-a-date"},
		{"20250101"},
		{"2025/01/01"},
		{""},
	}
	for _, tc := range cases {
		t.Run(tc.date, func(t *testing.T) {
			var out bytes.Buffer
			var errOut bytes.Buffer
			std := IO{Out: &out, Err: &errOut}
			code := auditRead([]string{"--date", tc.date}, std)
			if code != ExitUsage {
				t.Fatalf("date %q: exit %d; want %d (ExitUsage)", tc.date, code, ExitUsage)
			}
			if out.Len() != 0 {
				t.Fatalf("date %q: stdout not empty: %q", tc.date, out.String())
			}
		})
	}
}
