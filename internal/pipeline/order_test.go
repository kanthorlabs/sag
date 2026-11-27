package pipeline

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/kanthorlabs/sag/internal/audit"
	"github.com/kanthorlabs/sag/internal/core"
	"github.com/kanthorlabs/sag/internal/inventory"
)

type stageLog struct {
	names []string
}

func (l *stageLog) add(name string) {
	l.names = append(l.names, name)
}

type orderedResource struct {
	log    *stageLog
	result core.Result
	err    error
}

func (r *orderedResource) Preflight(context.Context) (core.PreflightReport, error) {
	return core.PreflightReport{Passed: true}, nil
}

func (r *orderedResource) Execute(context.Context, string, string, json.RawMessage, core.Limits) (core.Result, error) {
	r.log.add("execute")
	return r.result, r.err
}

func (r *orderedResource) Close() error { return nil }

func recordingStages(p *Pipeline, log *stageLog, records *[]audit.Record, appendErr error) {
	p.stages = stages{
		resolveLimits: func(e inventory.Entry, _ Request) (core.Limits, error) {
			log.add("resolveLimits")
			return e.Limits, nil
		},
		acquire: func(string) (func(), error) {
			log.add("acquire")
			return func() { log.add("release") }, nil
		},
		postexec: func(res core.Result, _ core.Limits, _ []string) core.Result {
			log.add("postexec")
			return res
		},
		scrub: func(err error) error {
			log.add("scrub")
			return err
		},
		audit: func(rec audit.Record) error {
			log.add("audit")
			*records = append(*records, rec)
			return appendErr
		},
	}
}

func orderedPipeline(t *testing.T, res core.Resource, appendErr error) (*Pipeline, *stageLog, *[]audit.Record) {
	t.Helper()
	log := &stageLog{}
	var records []audit.Record
	e := entry("main", inventory.StatusServed, res, "ac27")
	p := New(Options{Inventory: realInventory(t, e)})
	recordingStages(p, log, &records, appendErr)
	if r, ok := res.(*orderedResource); ok {
		r.log = log
	}
	return p, log, &records
}

func TestStageOrder(t *testing.T) {
	cases := []struct {
		name string
		res  *orderedResource
		want []string
	}{
		{
			name: "success",
			res:  &orderedResource{result: core.Result{Rows: [][]any{{int64(1)}}}},
			want: []string{"resolveLimits", "acquire", "execute", "postexec", "release", "audit"},
		},
		{
			name: "execute error",
			res:  &orderedResource{err: core.NewError(core.CodeUpstreamError, "upstream error, SQLSTATE 25006")},
			want: []string{"resolveLimits", "acquire", "execute", "release", "scrub", "audit"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, log, records := orderedPipeline(t, tc.res, nil)
			_, _ = p.Execute(context.Background(), request("ac27", "main"))
			if !reflect.DeepEqual(log.names, tc.want) {
				t.Fatalf("stages = %v; want %v", log.names, tc.want)
			}
			if len(*records) != 1 {
				t.Fatalf("audit records = %d; want 1", len(*records))
			}
		})
	}
}

func TestStageOrderDenyBeforeExecute(t *testing.T) {
	p, log, records := orderedPipeline(t, &orderedResource{}, nil)
	_, err := p.Execute(context.Background(), request("ac27", "missing"))
	assertCode(t, err, core.CodeNotFound)
	if want := []string{"scrub", "audit"}; !reflect.DeepEqual(log.names, want) {
		t.Fatalf("stages = %v; want %v", log.names, want)
	}
	if got := (*records)[0]; got.Decision != audit.DecisionDeny || got.Reason != string(core.CodeNotFound) {
		t.Fatalf("record = %+v", got)
	}
}

func TestAuditFailureReturnsInternal(t *testing.T) {
	appendErr := errors.New("read-only file system")
	cases := map[string]*orderedResource{
		"success":       {result: core.Result{Columns: []core.Column{{Name: "n", Type: "int4"}}, Rows: [][]any{{int64(1)}}}},
		"execute error": {err: core.NewError(core.CodeUpstreamError, "upstream error, SQLSTATE 25006")},
	}
	for name, res := range cases {
		t.Run(name, func(t *testing.T) {
			p, _, _ := orderedPipeline(t, res, appendErr)
			got, err := p.Execute(context.Background(), request("ac27", "main"))
			ce := assertCode(t, err, core.CodeInternal)
			if ce.Message != "internal error" {
				t.Fatalf("message = %q", ce.Message)
			}
			if !errors.Is(err, appendErr) {
				t.Fatalf("cause = %v; want the append error", ce.Cause)
			}
			if got.Rows != nil || got.Columns != nil || got.Truncated {
				t.Fatalf("result = %#v; want no data", got)
			}
		})
	}
	t.Run("reject", func(t *testing.T) {
		p, _, _ := orderedPipeline(t, &orderedResource{}, appendErr)
		err := p.Reject(request("ac27", "main"), core.NewError(core.CodeInvalidArgument, "bad arguments"))
		assertCode(t, err, core.CodeInternal)
	})
}

type rejection struct{}

func (rejection) Error() string  { return "statement rejected: SelectStmt.locking_clause" }
func (rejection) Reason() string { return "SelectStmt.locking_clause" }

func TestAuditRecordFields(t *testing.T) {
	trace := core.Trace{Input: "SELECT 1", InputSHA256: "in-sha", RenderedSHA256: "out-sha"}
	ok := &orderedResource{result: core.Result{Rows: [][]any{{int64(1)}, {int64(2)}}, Truncated: true, TruncatedReason: core.TruncatedMaxRows, Bytes: 42, Trace: trace}}
	p, _, records := orderedPipeline(t, ok, nil)
	now := time.Date(2026, 11, 25, 10, 0, 0, 123e6, time.UTC)
	p.opts.Now = func() time.Time { return now }
	req := request("ac27", "main")
	req.JTI = "01JTI"
	req.RemoteAddr = "10.0.0.1:5000"
	if _, err := p.Execute(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	got := (*records)[0]
	want := audit.Record{
		TS:             audit.Timestamp(now),
		RequestID:      req.ID,
		RemoteAddr:     "10.0.0.1:5000",
		Sub:            ptr("ac27"),
		JTI:            ptr("01JTI"),
		Resource:       "main",
		Capability:     "sql",
		Operation:      "query",
		SQL:            "SELECT 1",
		SQLSHA256:      "in-sha",
		RenderedSHA256: "out-sha",
		Decision:       audit.DecisionAllow,
		Rows:           2,
		Bytes:          42,
		Truncated:      true,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("record = %+v\nwant     %+v", got, want)
	}

	denied := &orderedResource{err: &core.Error{Code: core.CodeQueryRejected, Message: "statement rejected: SelectStmt.locking_clause", Trace: trace, Cause: rejection{}}}
	p, _, records = orderedPipeline(t, denied, nil)
	if _, err := p.Execute(context.Background(), request("ac27", "main")); err == nil {
		t.Fatal("want error")
	}
	got = (*records)[0]
	if got.Decision != audit.DecisionDeny || got.Reason != "query_rejected:SelectStmt.locking_clause" || got.SQL != "SELECT 1" || got.RenderedSHA256 != "out-sha" || got.Rows != 0 {
		t.Fatalf("deny record = %+v", got)
	}
	if got.Sub == nil || *got.Sub != "ac27" || got.JTI != nil || got.AuthCause != nil {
		t.Fatalf("deny record claims = %+v", got)
	}
}

func TestRejectAuditsDeny(t *testing.T) {
	p, log, records := orderedPipeline(t, &orderedResource{}, nil)
	want := core.NewError(core.CodeInvalidArgument, "bad arguments")
	err := p.Reject(request("ac27", "main"), want)
	if ce := assertCode(t, err, core.CodeInvalidArgument); ce.Message != want.Message {
		t.Fatalf("message = %q", ce.Message)
	}
	if !slices.Equal(log.names, []string{"scrub", "audit"}) {
		t.Fatalf("stages = %v", log.names)
	}
	if got := (*records)[0]; got.Decision != audit.DecisionDeny || got.Reason != string(core.CodeInvalidArgument) || got.Resource != "main" {
		t.Fatalf("record = %+v", got)
	}
}

func ptr(s string) *string {
	return &s
}

func TestListedAuditsAllow(t *testing.T) {
	p, log, records := orderedPipeline(t, &orderedResource{}, nil)
	req := Request{ID: "01LIST", Sub: "ac26", JTI: "01JTI", RemoteAddr: "10.0.0.1:5000", Operation: "list_resources"}
	if err := p.Listed(req, 3); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(log.names, []string{"audit"}) {
		t.Fatalf("stages = %v", log.names)
	}
	want := audit.Record{
		RequestID:  "01LIST",
		RemoteAddr: "10.0.0.1:5000",
		Sub:        ptr("ac26"),
		JTI:        ptr("01JTI"),
		Operation:  "list_resources",
		Decision:   audit.DecisionAllow,
		Rows:       3,
	}
	got := (*records)[0]
	got.TS, got.DurationMS = audit.Timestamp{}, 0
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("record = %+v; want %+v", got, want)
	}
}

func TestListedAuditFailureReturnsInternal(t *testing.T) {
	p, _, _ := orderedPipeline(t, &orderedResource{}, errors.New("read-only file system"))
	err := p.Listed(Request{ID: "01LIST", Sub: "ac27", Operation: "list_resources"}, 1)
	if ce := assertCode(t, err, core.CodeInternal); ce.Message != "internal error" {
		t.Fatalf("message = %q", ce.Message)
	}
}
