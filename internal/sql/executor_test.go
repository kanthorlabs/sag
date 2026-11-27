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
		if !f.ev.has("pool:dedicated") || f.ev.has("pool:acquire") {
			t.Fatalf("Preflight did not use a dedicated connection: %v", f.ev.list)
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

func (p *connPool) Dedicated(ctx context.Context) (Conn, error) {
	p.ev.add("pool:dedicated")
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

	t.Run("acquire errors", func(t *testing.T) {
		cases := []struct {
			name    string
			err     *core.Error
			code    core.Code
			message string
		}{
			{"connect", &core.Error{Code: core.CodeResourceUnavailable, Message: "preflight connect failed"}, core.CodeResourceUnavailable, "resource unavailable"},
			{"tls", &core.Error{Code: core.CodeResourceUnavailable, Message: "preflight TLS handshake failed"}, core.CodeResourceUnavailable, "resource unavailable"},
			{"pool wait", &core.Error{Code: core.CodeTimeout, Message: "timed out waiting for a connection"}, core.CodeTimeout, "request timed out"},
		}
		for _, tc := range cases {
			f := newFixture()
			f.pool.acquireErr = tc.err
			_, err := f.query(t, testText, defaultLimits())
			if codeOf(t, err) != tc.code || messageOf(t, err) != tc.message {
				t.Fatalf("%s: err = %v; want %s %q", tc.name, err, tc.code, tc.message)
			}
			var ce *core.Error
			if errors.As(err, &ce); !errors.Is(ce.Cause, tc.err) {
				t.Fatalf("%s: cause = %v; want the Acquire error", tc.name, ce.Cause)
			}
			if got := connEvents(f.ev); len(got) != 0 {
				t.Fatalf("%s: connection events without a connection: %v", tc.name, got)
			}
		}
	})

	t.Run("fixed rejection message", func(t *testing.T) {
		for _, sc := range failureScenarios() {
			if sc.code != core.CodeQueryRejected {
				continue
			}
			f := newFixture()
			sc.setup(f)
			_, err := f.query(t, testText, defaultLimits())
			if got := messageOf(t, err); got != "statement rejected" {
				t.Fatalf("%s: message = %q; want %q", sc.name, got, "statement rejected")
			}
		}
	})

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

func TestSQLSizeLimit(t *testing.T) {
	chain := func(n int) string {
		return "SELECT 1" + strings.Repeat("+1", n)
	}
	atLimit := chain((MaxSQLBytes - len("SELECT 1")) / 2)
	if len(atLimit) != MaxSQLBytes {
		t.Fatalf("chain length = %d; want %d", len(atLimit), MaxSQLBytes)
	}
	f := newFixture()
	if _, err := f.query(t, atLimit, defaultLimits()); err != nil {
		t.Fatalf("query at the limit: %v", err)
	}
	if !f.ev.has("dialect:parse:" + atLimit) {
		t.Fatal("query at the limit did not reach Parse")
	}
	for _, text := range []string{atLimit + "1", chain(22000)} {
		f := newFixture()
		_, err := f.query(t, text, defaultLimits())
		if codeOf(t, err) != core.CodeInvalidArgument || messageOf(t, err) != "sql exceeds 16384 bytes" {
			t.Fatalf("query of %d bytes: err = %v; want invalid_argument", len(text), err)
		}
		if len(f.ev.list) != 0 {
			t.Fatalf("query of %d bytes reached the dialect or the pool: %v", len(text), f.ev.list[0])
		}
	}
	f = newFixture()
	raw := []byte(`{"resource":"r","schema":"` + strings.Repeat("s", MaxSQLBytes) + `"}`)
	_, err := f.executor().Execute(context.Background(), CapabilityName, OperationListTables, raw, defaultLimits())
	if codeOf(t, err) != core.CodeInvalidArgument || len(f.ev.list) != 0 {
		t.Fatalf("list_tables with a long schema: err = %v, events %v; want invalid_argument before Parse", err, f.ev.list)
	}
}

type panicDialect struct {
	*fakeDialect
}

func (panicDialect) ValidateCatalog(context.Context, CatalogReader, Tree) error {
	panic("catalog walk")
}

func TestPanicClosesConnection(t *testing.T) {
	f := newFixture()
	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("no panic")
			}
		}()
		raw := []byte(`{"resource":"r","sql":"SELECT 1"}`)
		_, _ = NewExecutor(panicDialect{f.dialect}, f.pool).Execute(context.Background(), CapabilityName, OperationQuery, raw, defaultLimits())
	}()
	if f.ev.count("conn:close") != 1 || f.ev.has("conn:release") {
		t.Fatalf("connection not closed after a panic: %v", f.ev.list)
	}
}

func TestTruncatedReadWithoutCursorClosesConnection(t *testing.T) {
	f := newFixture()
	rows := &fakeRows{ev: f.ev, values: [][]any{{int64(1)}, {int64(2)}, {int64(3)}}}
	f.conn.rows[testRendered] = rows
	res, err := f.query(t, testText, core.Limits{MaxRows: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Rows) != 2 || !res.Truncated || res.TruncatedReason != core.TruncatedMaxRows {
		t.Fatalf("result = %+v; want 2 rows truncated by max_rows", res)
	}
	if !f.ev.has("rows:abort") || f.ev.index("rows:abort") > f.ev.index("conn:exec:"+testRollback) {
		t.Fatalf("rows not aborted before ROLLBACK: %v", f.ev.list)
	}
	if f.ev.count("conn:exec:"+testRollback) != 1 || f.ev.count("conn:close") != 1 || f.ev.has("conn:release") {
		t.Fatalf("truncated read without a cursor did not ROLLBACK and close: %v", f.ev.list)
	}

	f = newFixture()
	f.dialect.plan = RowCapPlan{Statement: testStmt, Fetch: testFetch}
	f.conn.rows[testFetch] = &fakeRows{ev: f.ev, values: [][]any{{int64(1)}, {int64(2)}, {int64(3)}}}
	if _, err := f.query(t, testText, core.Limits{MaxRows: 2}); err != nil {
		t.Fatal(err)
	}
	if f.ev.has("rows:abort") || f.ev.count("conn:release") != 1 {
		t.Fatalf("truncated cursor read did not release: %v", f.ev.list)
	}

	f = newFixture()
	f.conn.rows[testRendered] = &fakeRows{ev: f.ev, values: [][]any{{int64(1)}, {int64(2)}, {int64(3)}}}
	f.conn.execErr[testRollback] = &EngineError{SQLState: "08006", Class: Connect}
	if _, err := f.query(t, testText, core.Limits{MaxRows: 2}); codeOf(t, err) != core.CodeResourceUnavailable {
		t.Fatalf("truncated read with a failed ROLLBACK: err = %v; want resource_unavailable", err)
	}
}

type deadlinePool struct {
	*fakePool
	deadline *time.Duration
}

func (p deadlinePool) Acquire(ctx context.Context) (Conn, error) {
	if d, ok := ctx.Deadline(); ok {
		*p.deadline = time.Until(d)
	}
	return p.fakePool.Acquire(ctx)
}

func TestGoDeadlineHasSlack(t *testing.T) {
	f := newFixture()
	var got time.Duration
	raw := []byte(`{"resource":"r","sql":"SELECT 1"}`)
	if _, err := NewExecutor(f.dialect, deadlinePool{f.pool, &got}).Execute(context.Background(), CapabilityName, OperationQuery, raw, core.Limits{MaxRows: 1, Timeout: time.Second}); err != nil {
		t.Fatal(err)
	}
	if got <= time.Second+timeoutSlack-time.Second/2 || got > time.Second+timeoutSlack {
		t.Fatalf("Go deadline = %s; want the limit plus %s", got, timeoutSlack)
	}
}
