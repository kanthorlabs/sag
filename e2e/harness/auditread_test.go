package harness

import (
	"slices"
	"testing"
	"time"
)

func TestAuditDates(t *testing.T) {
	start := time.Date(2026, 11, 27, 23, 50, 0, 0, time.UTC)
	got := auditDates(start, start.Add(20*time.Minute))
	if !slices.Equal(got, []string{"2026-11-27", "2026-11-28"}) {
		t.Fatalf("auditDates over midnight = %v", got)
	}
	got = auditDates(start, start.Add(time.Minute))
	if !slices.Equal(got, []string{"2026-11-27"}) {
		t.Fatalf("auditDates in one day = %v", got)
	}
}

func TestCompleteAuditLinesDropsPartialLast(t *testing.T) {
	got, err := completeAuditLines("{\"a\":1}\n{\"b\":2}\n{\"c\":")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || string(got[1]) != `{"b":2}` {
		t.Fatalf("records = %q; want the two complete lines", got)
	}
	if _, err := completeAuditLines("{\"a\":1}\nnot json\n"); err == nil {
		t.Fatal("a complete line that is not JSON: want an error")
	}
	if got, err := completeAuditLines(""); err != nil || len(got) != 0 {
		t.Fatalf("empty output = %q, %v", got, err)
	}
}
