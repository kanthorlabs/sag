package daemon

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/oklog/ulid/v2"

	"github.com/kanthorlabs/sag/internal/audit"
)

type recordingWriter struct {
	records []audit.Record
	err     error
}

func (w *recordingWriter) Append(rec audit.Record) error {
	w.records = append(w.records, rec)
	return w.err
}

func TestAuthAuditHookAppendsFailure(t *testing.T) {
	w := &recordingWriter{}
	var log strings.Builder
	authAuditHook(w, slog.New(slog.NewJSONHandler(&log, nil)))(context.Background(), "10.0.0.1:5000", "expired")
	if len(w.records) != 1 {
		t.Fatalf("records = %d; want 1", len(w.records))
	}
	rec := w.records[0]
	if rec.AuthCause == nil || *rec.AuthCause != "expired" || rec.RemoteAddr != "10.0.0.1:5000" || rec.Sub != nil || rec.Decision != audit.DecisionDeny {
		t.Fatalf("record = %+v", rec)
	}
	if _, err := ulid.ParseStrict(rec.RequestID); err != nil {
		t.Fatalf("request_id %q is not a ULID", rec.RequestID)
	}
	if log.Len() != 0 {
		t.Fatalf("log = %s; want nothing on success", log.String())
	}

	w.err = errors.New("read-only file system")
	authAuditHook(w, slog.New(slog.NewJSONHandler(&log, nil)))(context.Background(), "10.0.0.1:5000", "expired")
	if !strings.Contains(log.String(), "audit append failed") {
		t.Fatalf("log = %s; want the append failure", log.String())
	}
}
