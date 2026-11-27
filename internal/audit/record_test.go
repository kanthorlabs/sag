package audit

import (
	"encoding/json"
	"testing"
	"time"
)

func TestRecordHasEveryPRDField(t *testing.T) {
	data, err := json.Marshal(Record{TS: Timestamp(time.Date(2026, 11, 25, 10, 0, 0, 7e6, time.FixedZone("x", 3600)))})
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	fields := []string{"ts", "request_id", "remote_addr", "sub", "jti", "auth_cause", "resource", "capability", "operation", "sql", "sql_sha256", "rendered_sha256", "decision", "reason", "rows", "bytes", "truncated", "duration_ms"}
	if len(got) != len(fields) {
		t.Fatalf("record has %d keys; want %d: %s", len(got), len(fields), data)
	}
	for _, f := range fields {
		if _, ok := got[f]; !ok {
			t.Fatalf("record has no key %q: %s", f, data)
		}
	}
	if got["ts"] != "2026-11-25T09:00:00.007Z" {
		t.Fatalf("ts = %v", got["ts"])
	}
	if got["sub"] != nil || got["jti"] != nil || got["auth_cause"] != nil {
		t.Fatalf("unset claims must encode as null: %s", data)
	}
}

func TestAuthFailureRecord(t *testing.T) {
	rec := AuthFailure(time.Unix(0, 0), "01REQ", "10.0.0.1:5000", "expired")
	if rec.Sub != nil || rec.JTI != nil || rec.AuthCause == nil || *rec.AuthCause != "expired" {
		t.Fatalf("claims = %+v", rec)
	}
	if rec.Decision != DecisionDeny || rec.Reason != "unauthenticated" || rec.RequestID != "01REQ" || rec.RemoteAddr != "10.0.0.1:5000" {
		t.Fatalf("record = %+v", rec)
	}
}
