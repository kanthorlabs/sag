package sql

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kanthorlabs/sag/internal/core"
)

const (
	testText     = "SELECT 1"
	testRendered = "R(SELECT 1)"
	testStmt     = "DECLARE c CURSOR FOR R(SELECT 1)"
	testFetch    = "FETCH 101 FROM c"
)

type scenario struct {
	name  string
	setup func(f *fixture)
	code  core.Code
}

func failureScenarios() []scenario {
	readOnly := &EngineError{SQLState: "25006", Class: ReadOnlyViolation}
	return []scenario{
		{"request begin fails", func(f *fixture) {
			f.conn.execErr[testBegin] = &EngineError{SQLState: "08006", Class: Connect}
		}, core.CodeResourceUnavailable},
		{"catalog rejects", func(f *fixture) {
			f.dialect.catalogErr = errors.New("function not allowed")
		}, core.CodeQueryRejected},
		{"catalog engine timeout", func(f *fixture) {
			f.dialect.catalogErr = &EngineError{SQLState: "57014", Class: Timeout}
		}, core.CodeTimeout},
		{"plan fails", func(f *fixture) {
			f.dialect.planErr = errors.New("no plan")
		}, core.CodeQueryRejected},
		{"statement fails", func(f *fixture) {
			f.dialect.plan = RowCapPlan{Statement: testStmt, Fetch: testFetch}
			f.conn.execErr[testStmt] = readOnly
		}, core.CodeUpstreamError},
		{"fetch fails", func(f *fixture) {
			f.dialect.plan = RowCapPlan{Statement: testStmt, Fetch: testFetch}
			f.conn.queryErr[testFetch] = &EngineError{SQLState: "55P03", Class: Timeout}
		}, core.CodeTimeout},
		{"query fails", func(f *fixture) {
			f.conn.queryErr[testRendered] = readOnly
		}, core.CodeUpstreamError},
		{"rows error", func(f *fixture) {
			f.conn.rows[testRendered] = &fakeRows{values: [][]any{{int64(1)}}, err: &EngineError{SQLState: "XX000", Class: Other}}
		}, core.CodeUpstreamError},
		{"rollback fails", func(f *fixture) {
			f.conn.execErr[testRollback] = &EngineError{SQLState: "08006", Class: Connect}
		}, core.CodeResourceUnavailable},
	}
}

func TestStaticValidationBeforeConnection(t *testing.T) {
	cases := []struct {
		name  string
		setup func(f *fixture)
	}{
		{"parse fails", func(f *fixture) { f.dialect.parseErr = errors.New("syntax error") }},
		{"static rejects input", func(f *fixture) {
			f.dialect.staticErr = func(Tree) error { return errors.New("node not allowed") }
		}},
		{"render fails", func(f *fixture) { f.dialect.renderErr = errors.New("render failed") }},
		{"static rejects rendered", func(f *fixture) {
			f.dialect.staticErr = func(t Tree) error {
				if t.(string) == testRendered {
					return errors.New("node not allowed")
				}
				return nil
			}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture()
			tc.setup(f)
			_, err := f.query(t, testText, defaultLimits())
			if got := codeOf(t, err); got != core.CodeQueryRejected {
				t.Fatalf("code = %s, want %s", got, core.CodeQueryRejected)
			}
			if f.ev.has("pool:acquire") {
				t.Fatalf("connection taken after static rejection: %v", f.ev.list)
			}
		})
	}

	t.Run("order", func(t *testing.T) {
		f := newFixture()
		if _, err := f.query(t, testText, defaultLimits()); err != nil {
			t.Fatal(err)
		}
		want := []string{
			"dialect:parse:" + testText,
			"dialect:validateStatic:" + testText,
			"dialect:render",
			"dialect:parse:" + testRendered,
			"dialect:validateStatic:" + testRendered,
			"pool:acquire",
			"dialect:requestBegin",
			"conn:exec:" + testBegin,
			"dialect:validateCatalog:" + testRendered,
			"dialect:planRowCap:" + testRendered,
			"conn:query:" + testRendered,
			"rows:close",
			"conn:exec:" + testRollback,
			"conn:release",
		}
		if !slices.Equal(f.ev.list, want) {
			t.Fatalf("events:\n got %v\nwant %v", f.ev.list, want)
		}
	})
}

func TestRollbackAlways(t *testing.T) {
	all := append(failureScenarios(), scenario{"success", func(*fixture) {}, ""})
	for _, sc := range all {
		t.Run(sc.name, func(t *testing.T) {
			f := newFixture()
			sc.setup(f)
			_, err := f.query(t, testText, defaultLimits())
			if sc.code == "" && err != nil {
				t.Fatal(err)
			}
			if sc.code != "" {
				if got := codeOf(t, err); got != sc.code {
					t.Fatalf("code = %s, want %s (%v)", got, sc.code, err)
				}
			}
			ce := connEvents(f.ev)
			if n := f.ev.count("conn:exec:" + testRollback); n != 1 {
				t.Fatalf("ROLLBACK sent %d times: %v", n, ce)
			}
			last := ce[len(ce)-1]
			if last != "conn:release" && last != "conn:close" {
				t.Fatalf("last connection event = %s", last)
			}
			if ce[len(ce)-2] != "conn:exec:"+testRollback {
				t.Fatalf("ROLLBACK is not the last statement: %v", ce)
			}
		})
	}

	t.Run("RunReadOnly", func(t *testing.T) {
		f := newFixture()
		err := RunReadOnly(context.Background(), f.dialect, f.pool, core.Limits{Timeout: CatalogTimeout}, func(ctx context.Context, r CatalogReader) error {
			_, err := r.Query(ctx, "SELECT catalog")
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		want := []string{"conn:exec:" + testBegin, "conn:query:SELECT catalog", "conn:exec:" + testRollback, "conn:release"}
		if got := connEvents(f.ev); !slices.Equal(got, want) {
			t.Fatalf("events:\n got %v\nwant %v", got, want)
		}
	})

	t.Run("RunReadOnly fn fails", func(t *testing.T) {
		f := newFixture()
		cause := errors.New("catalog read failed")
		err := RunReadOnly(context.Background(), f.dialect, f.pool, core.Limits{Timeout: CatalogTimeout}, func(context.Context, CatalogReader) error {
			return cause
		})
		if !errors.Is(err, cause) {
			t.Fatalf("err = %v, want %v", err, cause)
		}
		want := []string{"conn:exec:" + testBegin, "conn:exec:" + testRollback, "conn:close"}
		if got := connEvents(f.ev); !slices.Equal(got, want) {
			t.Fatalf("events:\n got %v\nwant %v", got, want)
		}
	})

	t.Run("rollback runs after request deadline", func(t *testing.T) {
		f := newFixture()
		var rollbackCtxErr error
		conn := &deadlineConn{fakeConn: f.conn, onRollback: func(ctx context.Context) { rollbackCtxErr = ctx.Err() }}
		pool := &connPool{ev: f.ev, conn: conn}
		err := RunReadOnly(context.Background(), f.dialect, pool, core.Limits{Timeout: time.Millisecond}, func(ctx context.Context, r CatalogReader) error {
			<-ctx.Done()
			return ctx.Err()
		})
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("err = %v", err)
		}
		if rollbackCtxErr != nil {
			t.Fatalf("ROLLBACK context already done: %v", rollbackCtxErr)
		}
		if !f.ev.has("conn:close") {
			t.Fatalf("connection not closed: %v", f.ev.list)
		}
	})

	t.Run("Preflight", func(t *testing.T) {
		f := newFixture()
		f.dialect.preflightReport = core.PreflightReport{Passed: true}
		report, err := f.executor().Preflight(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if !report.Passed {
			t.Fatal("report not passed")
		}
		want := []string{"conn:exec:" + testBegin, "conn:exec:" + testRollback, "conn:release"}
		if got := connEvents(f.ev); !slices.Equal(got, want) {
			t.Fatalf("events:\n got %v\nwant %v", got, want)
		}
	})

	t.Run("acquire fails", func(t *testing.T) {
		f := newFixture()
		f.pool.acquireErr = core.NewError(core.CodeTimeout, "pool exhausted")
		_, err := f.query(t, testText, defaultLimits())
		if got := codeOf(t, err); got != core.CodeTimeout {
			t.Fatalf("code = %s", got)
		}
		if got := connEvents(f.ev); len(got) != 0 {
			t.Fatalf("connection events without a connection: %v", got)
		}
	})
}

type deadlineConn struct {
	*fakeConn
	onRollback func(ctx context.Context)
}

func (c *deadlineConn) Exec(ctx context.Context, text string) error {
	if text == testRollback {
		c.onRollback(ctx)
	}
	return c.fakeConn.Exec(ctx, text)
}

type connPool struct {
	ev   *events
	conn Conn
}

func (p *connPool) Acquire(ctx context.Context) (Conn, error) {
	p.ev.add("pool:acquire")
	return p.conn, nil
}

func (p *connPool) Close() {}

var commitWord = regexp.MustCompile(`(?i)\bcommit\b`)

func TestNeverCommit(t *testing.T) {
	all := append(failureScenarios(), scenario{"success", func(*fixture) {}, ""})
	for _, sc := range all {
		f := newFixture()
		sc.setup(f)
		_, _ = f.query(t, testText, defaultLimits())
		for _, text := range f.conn.texts {
			if commitWord.MatchString(text) {
				t.Fatalf("%s: COMMIT sent: %q", sc.name, text)
			}
		}
	}

	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	checked := 0
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		src, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		checked++
		if loc := commitWord.FindIndex(src); loc != nil {
			t.Fatalf("%s holds COMMIT at byte %d", name, loc[0])
		}
	}
	if checked == 0 {
		t.Fatal("no source file checked")
	}
}

func TestCloseOnRollbackFailure(t *testing.T) {
	f := newFixture()
	f.conn.execErr[testRollback] = &EngineError{SQLState: "08006", Class: Connect}
	_, err := f.query(t, testText, defaultLimits())
	if err == nil {
		t.Fatal("no error after ROLLBACK failure")
	}
	if !f.ev.has("conn:close") || f.ev.has("conn:release") {
		t.Fatalf("connection not closed: %v", f.ev.list)
	}

	f = newFixture()
	f.conn.execErr[testRollback] = errors.New("rollback failed")
	err = RunReadOnly(context.Background(), f.dialect, f.pool, core.Limits{}, func(context.Context, CatalogReader) error { return nil })
	if err == nil {
		t.Fatal("RunReadOnly: no error after ROLLBACK failure")
	}
	if !f.ev.has("conn:close") || f.ev.has("conn:release") {
		t.Fatalf("RunReadOnly: connection not closed: %v", f.ev.list)
	}
}

func TestCloseOnError(t *testing.T) {
	for _, sc := range failureScenarios() {
		t.Run(sc.name, func(t *testing.T) {
			f := newFixture()
			sc.setup(f)
			_, err := f.query(t, testText, defaultLimits())
			if err == nil {
				t.Fatal("no error")
			}
			if f.ev.count("conn:close") != 1 || f.ev.has("conn:release") {
				t.Fatalf("connection not closed exactly once: %v", f.ev.list)
			}
			if i := f.ev.index("rows:close"); i >= 0 && i > f.ev.index("conn:exec:"+testRollback) {
				t.Fatalf("rows closed after ROLLBACK: %v", f.ev.list)
			}
		})
	}

	t.Run("success releases", func(t *testing.T) {
		f := newFixture()
		if _, err := f.query(t, testText, defaultLimits()); err != nil {
			t.Fatal(err)
		}
		if f.ev.count("conn:release") != 1 || f.ev.has("conn:close") {
			t.Fatalf("connection not released exactly once: %v", f.ev.list)
		}
	})

	t.Run("messages", func(t *testing.T) {
		f := newFixture()
		f.conn.queryErr[testRendered] = &EngineError{SQLState: "25006", Class: ReadOnlyViolation}
		_, err := f.query(t, testText, defaultLimits())
		if got, want := messageOf(t, err), "upstream error, SQLSTATE 25006"; got != want {
			t.Fatalf("message = %q, want %q", got, want)
		}
	})

	t.Run("trace on error", func(t *testing.T) {
		f := newFixture()
		f.conn.queryErr[testRendered] = &EngineError{SQLState: "25006", Class: ReadOnlyViolation}
		_, err := f.query(t, testText, defaultLimits())
		var ce *core.Error
		if !errors.As(err, &ce) {
			t.Fatal(err)
		}
		if ce.Trace != wantTrace() {
			t.Fatalf("trace = %+v", ce.Trace)
		}
	})
}

func wantTrace() core.Trace {
	in := sha256.Sum256([]byte(testText))
	out := sha256.Sum256([]byte(testRendered))
	return core.Trace{Input: testText, InputSHA256: hex.EncodeToString(in[:]), RenderedSHA256: hex.EncodeToString(out[:])}
}

func TestFetchEmptyReadsStatementRows(t *testing.T) {
	cols := []core.Column{{Name: "n", Type: "int8"}}

	t.Run("empty fetch", func(t *testing.T) {
		f := newFixture()
		f.conn.rows[testRendered] = &fakeRows{columns: cols, values: [][]any{{int64(1)}, {int64(2)}}}
		res, err := f.query(t, testText, defaultLimits())
		if err != nil {
			t.Fatal(err)
		}
		if f.ev.has("conn:exec:" + testRendered) {
			t.Fatalf("statement sent through Exec: %v", f.ev.list)
		}
		if len(res.Rows) != 2 || !slices.Equal(res.Columns, cols) {
			t.Fatalf("result = %+v", res)
		}
		if res.Trace != wantTrace() {
			t.Fatalf("trace = %+v", res.Trace)
		}
	})

	t.Run("fetch", func(t *testing.T) {
		f := newFixture()
		f.dialect.plan = RowCapPlan{Statement: testStmt, Fetch: testFetch}
		f.conn.rows[testFetch] = &fakeRows{columns: cols, values: [][]any{{int64(1)}}}
		res, err := f.query(t, testText, defaultLimits())
		if err != nil {
			t.Fatal(err)
		}
		want := []string{"conn:exec:" + testBegin, "conn:exec:" + testStmt, "conn:query:" + testFetch, "conn:exec:" + testRollback, "conn:release"}
		if got := connEvents(f.ev); !slices.Equal(got, want) {
			t.Fatalf("events:\n got %v\nwant %v", got, want)
		}
		if len(res.Rows) != 1 {
			t.Fatalf("rows = %v", res.Rows)
		}
	})

	t.Run("no rows encodes empty list", func(t *testing.T) {
		f := newFixture()
		res, err := f.query(t, testText, defaultLimits())
		if err != nil {
			t.Fatal(err)
		}
		if res.Rows == nil {
			t.Fatal("rows is nil")
		}
	})
}

func TestStopsAtMaxRowsPlusOne(t *testing.T) {
	cases := []struct {
		name      string
		rows      int
		maxRows   int
		wantRows  int
		truncated bool
		maxNext   int
	}{
		{"more rows than max", 10, 3, 3, true, 4},
		{"exactly max", 3, 3, 3, false, 4},
		{"fewer rows", 2, 3, 2, false, 3},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture()
			values := make([][]any, tc.rows)
			for i := range values {
				values[i] = []any{int64(i)}
			}
			rows := &fakeRows{values: values}
			f.conn.rows[testRendered] = rows
			res, err := f.query(t, testText, core.Limits{MaxRows: tc.maxRows})
			if err != nil {
				t.Fatal(err)
			}
			if len(res.Rows) != tc.wantRows || res.Truncated != tc.truncated {
				t.Fatalf("rows = %d truncated = %v", len(res.Rows), res.Truncated)
			}
			if tc.truncated && res.TruncatedReason != core.TruncatedMaxRows {
				t.Fatalf("reason = %q", res.TruncatedReason)
			}
			if !tc.truncated && res.TruncatedReason != core.TruncatedNone {
				t.Fatalf("reason = %q", res.TruncatedReason)
			}
			if rows.nextN > tc.maxNext || rows.valuesN > tc.maxRows {
				t.Fatalf("read past max_rows+1: Next %d, Values %d", rows.nextN, rows.valuesN)
			}
			if !rows.closed {
				t.Fatal("rows not closed")
			}
		})
	}
}

func TestPlanRowCapGetsMaxRows(t *testing.T) {
	f := newFixture()
	var got int
	d := &maxRowsDialect{fakeDialect: f.dialect, got: &got}
	raw := []byte(`{"resource":"r","sql":"SELECT 1"}`)
	if _, err := NewExecutor(d, f.pool).Execute(context.Background(), CapabilityName, OperationQuery, raw, core.Limits{MaxRows: 42}); err != nil {
		t.Fatal(err)
	}
	if got != 42 {
		t.Fatalf("PlanRowCap maxRows = %d", got)
	}
}

type maxRowsDialect struct {
	*fakeDialect
	got *int
}

func (d *maxRowsDialect) PlanRowCap(t Tree, rendered string, maxRows int) (RowCapPlan, error) {
	*d.got = maxRows
	return d.fakeDialect.PlanRowCap(t, rendered, maxRows)
}

func TestExecuteRejectsBadInput(t *testing.T) {
	cases := []struct {
		name       string
		capability string
		operation  string
		input      string
	}{
		{"unknown capability", "logs", OperationQuery, `{"sql":"SELECT 1"}`},
		{"unknown operation", CapabilityName, "drop", `{"sql":"SELECT 1"}`},
		{"empty sql", CapabilityName, OperationQuery, `{"sql":""}`},
		{"sql not a string", CapabilityName, OperationQuery, `{"sql":1}`},
		{"not json", CapabilityName, OperationQuery, `{`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture()
			_, err := f.executor().Execute(context.Background(), tc.capability, tc.operation, []byte(tc.input), defaultLimits())
			if got := codeOf(t, err); got != core.CodeInvalidArgument {
				t.Fatalf("code = %s", got)
			}
			if len(f.ev.list) != 0 {
				t.Fatalf("events after invalid input: %v", f.ev.list)
			}
		})
	}
}

func TestCloseClosesPool(t *testing.T) {
	f := newFixture()
	if err := f.executor().Close(); err != nil {
		t.Fatal(err)
	}
	if !f.ev.has("pool:close") {
		t.Fatal("pool not closed")
	}
}
