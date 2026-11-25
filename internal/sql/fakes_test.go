package sql

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/kanthorlabs/sag/internal/core"
)

type events struct {
	list []string
}

func (e *events) add(s string) {
	e.list = append(e.list, s)
}

func (e *events) has(s string) bool {
	for _, v := range e.list {
		if v == s {
			return true
		}
	}
	return false
}

func (e *events) count(s string) int {
	n := 0
	for _, v := range e.list {
		if v == s {
			n++
		}
	}
	return n
}

func (e *events) index(s string) int {
	for i, v := range e.list {
		if v == s {
			return i
		}
	}
	return -1
}

type fakeDialect struct {
	ev              *events
	begin           []string
	plan            RowCapPlan
	parseErr        error
	staticErr       func(t Tree) error
	renderErr       error
	catalogErr      error
	planErr         error
	preflightReport core.PreflightReport
	preflightErr    error
}

func (d *fakeDialect) EngineGuard() EngineGuard { return EngineGuardTransaction }
func (d *fakeDialect) SessionGuard() []string   { return nil }
func (d *fakeDialect) ViolationCodes() []string { return []string{"25006"} }

func (d *fakeDialect) RequestBegin(lim core.Limits) []string {
	d.ev.add("dialect:requestBegin")
	return d.begin
}

func (d *fakeDialect) Parse(text string) (Tree, error) {
	d.ev.add("dialect:parse:" + text)
	if d.parseErr != nil {
		return nil, d.parseErr
	}
	return text, nil
}

func (d *fakeDialect) ValidateStatic(t Tree) error {
	d.ev.add("dialect:validateStatic:" + t.(string))
	if d.staticErr != nil {
		return d.staticErr(t)
	}
	return nil
}

func (d *fakeDialect) Render(t Tree) (string, error) {
	d.ev.add("dialect:render")
	if d.renderErr != nil {
		return "", d.renderErr
	}
	return "R(" + t.(string) + ")", nil
}

func (d *fakeDialect) ValidateCatalog(ctx context.Context, tx CatalogReader, t Tree) error {
	d.ev.add("dialect:validateCatalog:" + t.(string))
	return d.catalogErr
}

func (d *fakeDialect) PlanRowCap(t Tree, rendered string, maxRows int) (RowCapPlan, error) {
	d.ev.add("dialect:planRowCap:" + rendered)
	if d.planErr != nil {
		return RowCapPlan{}, d.planErr
	}
	if d.plan.Statement == "" && d.plan.Fetch == "" {
		return RowCapPlan{Statement: rendered}, nil
	}
	return d.plan, nil
}

func (d *fakeDialect) Preflight(ctx context.Context, conn CatalogReader) (core.PreflightReport, error) {
	d.ev.add("dialect:preflight")
	return d.preflightReport, d.preflightErr
}

type fakePool struct {
	ev         *events
	conn       *fakeConn
	acquireErr error
}

func (p *fakePool) Acquire(ctx context.Context) (Conn, error) {
	p.ev.add("pool:acquire")
	if p.acquireErr != nil {
		return nil, p.acquireErr
	}
	return p.conn, nil
}

func (p *fakePool) Close() {
	p.ev.add("pool:close")
}

type fakeConn struct {
	ev       *events
	execErr  map[string]error
	queryErr map[string]error
	rows     map[string]*fakeRows
	texts    []string
}

func (c *fakeConn) Exec(ctx context.Context, text string) error {
	c.ev.add("conn:exec:" + text)
	c.texts = append(c.texts, text)
	return c.execErr[text]
}

func (c *fakeConn) ExecParams(ctx context.Context, text string, args ...any) error {
	c.ev.add("conn:execParams:" + text)
	c.texts = append(c.texts, text)
	return c.execErr[text]
}

func (c *fakeConn) Query(ctx context.Context, text string, args ...any) (Rows, error) {
	c.ev.add("conn:query:" + text)
	c.texts = append(c.texts, text)
	if err := c.queryErr[text]; err != nil {
		return nil, err
	}
	if r, ok := c.rows[text]; ok {
		return r, nil
	}
	return &fakeRows{ev: c.ev}, nil
}

func (c *fakeConn) Release() {
	c.ev.add("conn:release")
}

func (c *fakeConn) Close() error {
	c.ev.add("conn:close")
	return nil
}

type fakeRows struct {
	ev      *events
	columns []core.Column
	values  [][]any
	err     error
	pos     int
	nextN   int
	valuesN int
	closed  bool
}

func (r *fakeRows) Columns() []core.Column { return r.columns }

func (r *fakeRows) Next() bool {
	r.nextN++
	if r.pos >= len(r.values) {
		return false
	}
	r.pos++
	return true
}

func (r *fakeRows) Values() ([]any, error) {
	r.valuesN++
	return r.values[r.pos-1], nil
}

func (r *fakeRows) Err() error { return r.err }

func (r *fakeRows) Close() {
	r.closed = true
	if r.ev != nil {
		r.ev.add("rows:close")
	}
}

const (
	testBegin    = "BEGIN TEST READ ONLY"
	testRollback = "ROLLBACK"
)

type fixture struct {
	ev      *events
	dialect *fakeDialect
	pool    *fakePool
	conn    *fakeConn
}

func newFixture() *fixture {
	ev := &events{}
	conn := &fakeConn{ev: ev, execErr: map[string]error{}, queryErr: map[string]error{}, rows: map[string]*fakeRows{}}
	return &fixture{
		ev:      ev,
		dialect: &fakeDialect{ev: ev, begin: []string{testBegin}},
		pool:    &fakePool{ev: ev, conn: conn},
		conn:    conn,
	}
}

func (f *fixture) executor() core.Resource {
	return NewExecutor(f.dialect, f.pool)
}

func (f *fixture) query(t *testing.T, text string, lim core.Limits) (core.Result, error) {
	t.Helper()
	raw, err := json.Marshal(map[string]any{"resource": "r", "sql": text, "max_rows": 7})
	if err != nil {
		t.Fatal(err)
	}
	return f.executor().Execute(context.Background(), CapabilityName, OperationQuery, raw, lim)
}

func defaultLimits() core.Limits {
	return core.Limits{MaxRows: 100, Timeout: 0}
}

func codeOf(t *testing.T, err error) core.Code {
	t.Helper()
	var ce *core.Error
	if !errors.As(err, &ce) {
		t.Fatalf("error %v is not *core.Error", err)
	}
	return ce.Code
}

func messageOf(t *testing.T, err error) string {
	t.Helper()
	var ce *core.Error
	if !errors.As(err, &ce) {
		t.Fatalf("error %v is not *core.Error", err)
	}
	return ce.Message
}

func connEvents(ev *events) []string {
	var out []string
	for _, v := range ev.list {
		if strings.HasPrefix(v, "conn:") {
			out = append(out, v)
		}
	}
	return out
}
