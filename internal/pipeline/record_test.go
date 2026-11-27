package pipeline

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/kanthorlabs/sag/internal/audit"
	"github.com/kanthorlabs/sag/internal/core"
	"github.com/kanthorlabs/sag/internal/inventory"
	"github.com/kanthorlabs/sag/internal/scrub"
	"github.com/kanthorlabs/sag/internal/secret"
)

const loadedSecret = "s3cr3t-value-for-record"

type namedRejection string

func (n namedRejection) Error() string  { return "statement rejected: " + string(n) }
func (n namedRejection) Reason() string { return string(n) }

func TestAuditFieldsScrubbed(t *testing.T) {
	trace := core.Trace{Input: `SELECT "` + loadedSecret + `".f()`}
	message := "statement rejected: function_schema"
	denied := &orderedResource{err: &core.Error{Code: core.CodeQueryRejected, Message: message, Trace: trace, Cause: namedRejection("function_schema:" + loadedSecret + ".f")}}
	log := &stageLog{}
	denied.log = log
	var records []audit.Record
	p := New(Options{
		Inventory: realInventory(t, entry("main", inventory.StatusServed, denied, "ac24")),
		Scrubber:  scrub.New([]secret.Credential{secret.NewCredential("sag-main", []byte(loadedSecret))}),
	})
	recordingStages(p, log, &records, nil)
	p.stages.scrub = p.opts.Scrubber.Error

	_, err := p.Execute(context.Background(), request("ac24", "main"))
	if ce := assertCode(t, err, core.CodeQueryRejected); ce.Message != message {
		t.Fatalf("message = %q; want %q", ce.Message, message)
	}
	got := records[0]
	if got.SQL != `SELECT "[REDACTED]".f()` || got.Reason != "query_rejected:function_schema:[REDACTED].f" {
		t.Fatalf("record sql = %q, reason = %q; want the secret scrubbed", got.SQL, got.Reason)
	}
}

func TestAuditRecordBoundsNames(t *testing.T) {
	p, _, records := orderedPipeline(t, &orderedResource{}, nil)
	req := request("ac27", strings.Repeat("r", 100))
	req.Operation = strings.Repeat("o", 64)
	_ = p.Reject(req, core.NewError(core.CodeInvalidArgument, "bad arguments"))
	got := (*records)[0]
	if got.Resource != strings.Repeat("r", maxRecordName) || got.Operation != strings.Repeat("o", maxRecordName) {
		t.Fatalf("record resource = %q (%d bytes), operation = %q (%d bytes); want the first 63 bytes", got.Resource, len(got.Resource), got.Operation, len(got.Operation))
	}

	p, _, records = orderedPipeline(t, &orderedResource{}, nil)
	req = request("ac27", strings.Repeat("r", maxRecordName))
	_ = p.Reject(req, core.NewError(core.CodeInvalidArgument, "bad arguments"))
	if got := (*records)[0]; got.Resource != req.Resource || got.Operation != "query" {
		t.Fatalf("record = %+v; want names up to 63 bytes unchanged", got)
	}
}

func TestAuditAppendFailureLogged(t *testing.T) {
	appendErr := errors.New("no space left on device")
	calls := map[string]func(p *Pipeline) error{
		"execute": func(p *Pipeline) error {
			_, err := p.Execute(context.Background(), request("ac27", "main"))
			return err
		},
		"listed": func(p *Pipeline) error {
			return p.Listed(Request{ID: "req-1", Sub: "ac27", Operation: "list_resources"}, 1)
		},
	}
	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			var buf strings.Builder
			res := &orderedResource{}
			log := &stageLog{}
			res.log = log
			var records []audit.Record
			p := New(Options{
				Inventory: realInventory(t, entry("main", inventory.StatusServed, res, "ac27")),
				Log:       slog.New(slog.NewJSONHandler(&buf, nil)),
			})
			recordingStages(p, log, &records, appendErr)
			assertCode(t, call(p), core.CodeInternal)
			out := buf.String()
			for _, want := range []string{`"level":"ERROR"`, `"msg":"audit append failed"`, `"request_id":"req-1"`, `"error":"no space left on device"`} {
				if !strings.Contains(out, want) {
					t.Fatalf("log = %s; want %s", out, want)
				}
			}
		})
	}
}
