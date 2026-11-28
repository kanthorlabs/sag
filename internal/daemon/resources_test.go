package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	_ "github.com/kanthorlabs/sag/internal/adapter/postgres"
	"github.com/kanthorlabs/sag/internal/config"
	"github.com/kanthorlabs/sag/internal/core"
	"github.com/kanthorlabs/sag/internal/inventory"
	"github.com/kanthorlabs/sag/internal/scrub"
	"github.com/kanthorlabs/sag/internal/secret"
	"github.com/kanthorlabs/sag/internal/sql"
)

const fakeKind = "daemon-fake"

type fakeDialect struct{ sql.Dialect }

type fakePool struct{ closed bool }

func (p *fakePool) Acquire(context.Context) (sql.Conn, error) {
	return nil, core.NewError(core.CodeResourceUnavailable, "resource unavailable")
}

func (p *fakePool) Dedicated(ctx context.Context) (sql.Conn, error) {
	return p.Acquire(ctx)
}

func (p *fakePool) Close() { p.closed = true }

type fakeResource struct{ closed bool }

func (r *fakeResource) Preflight(context.Context) (core.PreflightReport, error) {
	return core.PreflightReport{Passed: true}, nil
}

func (r *fakeResource) Execute(context.Context, string, string, json.RawMessage, core.Limits) (core.Result, error) {
	return core.Result{}, nil
}

func (r *fakeResource) Close() error {
	r.closed = true
	return nil
}

type fakeAdapter struct {
	mu        sync.Mutex
	opened    int
	openedSQL int
	configs   []core.ResourceConfig
	creds     []string
	resource  *fakeResource
	dialect   *fakeDialect
	pool      *fakePool
	openErr   error
}

func (a *fakeAdapter) Kind() string { return fakeKind }

func (a *fakeAdapter) Capabilities() []core.Capability { return []core.Capability{sql.Capability()} }

func (a *fakeAdapter) Open(_ context.Context, cfg core.ResourceConfig, cred secret.Credential) (core.Resource, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.opened++
	a.configs = append(a.configs, cfg)
	a.creds = append(a.creds, string(cred.Reveal()))
	if a.openErr != nil {
		return nil, a.openErr
	}
	a.resource = &fakeResource{}
	return a.resource, nil
}

func (a *fakeAdapter) OpenSQL(_ context.Context, cfg core.ResourceConfig, cred secret.Credential) (sql.Dialect, sql.Pool, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.openedSQL++
	a.configs = append(a.configs, cfg)
	a.creds = append(a.creds, string(cred.Reveal()))
	if a.openErr != nil {
		return nil, nil, a.openErr
	}
	a.dialect = &fakeDialect{}
	a.pool = &fakePool{}
	return a.dialect, a.pool, nil
}

var registeredFake = &fakeAdapter{}

func init() {
	core.Register(registeredFake)
}

func resetFake(t *testing.T) *fakeAdapter {
	t.Helper()
	registeredFake.mu.Lock()
	defer registeredFake.mu.Unlock()
	registeredFake.opened, registeredFake.openedSQL = 0, 0
	registeredFake.configs, registeredFake.creds = nil, nil
	registeredFake.resource, registeredFake.dialect, registeredFake.pool = nil, nil, nil
	registeredFake.openErr = nil
	return registeredFake
}

type scheduled struct {
	name    string
	res     core.Resource
	timeout time.Duration
}

type unopened struct {
	name    string
	open    func(ctx context.Context) (core.Resource, error)
	timeout time.Duration
}

type fakeScheduler struct {
	added    []scheduled
	unopened []unopened
}

func (s *fakeScheduler) Add(name string, res core.Resource, timeout time.Duration) {
	s.added = append(s.added, scheduled{name: name, res: res, timeout: timeout})
}

func (s *fakeScheduler) AddUnopened(name string, open func(ctx context.Context) (core.Resource, error), timeout time.Duration) {
	s.unopened = append(s.unopened, unopened{name: name, open: open, timeout: timeout})
}

func (s *fakeScheduler) AddUnopenedImmediate(name string, open func(ctx context.Context) (core.Resource, error), timeout time.Duration) {
	s.unopened = append(s.unopened, unopened{name: name, open: open, timeout: timeout})
}

type fakeApprovals struct{}

func (fakeApprovals) Verify(string, string) error { return nil }

type openResult struct {
	inv    *inventory.Inventory
	set    *resourceSet
	opened []core.Resource
	sched  *fakeScheduler
	log    *strings.Builder
}

func openWith(t *testing.T, kind, dsn string, approvals core.ApprovalVerifier, wrap func(sql.Dialect) sql.Dialect) openResult {
	t.Helper()
	text := strings.Replace(testConfig, "kind: postgres", "kind: "+kind, 1) + "    limits:\n      connect_timeout: 1s\n"
	cfg, err := config.Parse([]byte(text))
	if err != nil {
		t.Fatal(err)
	}
	creds := map[string]secret.Credential{"sag-main": secret.NewCredential("sag-main", []byte(dsn))}
	r := openResult{inv: inventory.New(), sched: &fakeScheduler{}, log: &strings.Builder{}}
	r.set, err = openResources(context.Background(), slog.New(slog.NewJSONHandler(r.log, nil)), cfg, creds, nil, scrub.New(nil), r.inv, approvals, wrap, r.sched)
	t.Cleanup(func() { closeResources(slog.New(slog.NewJSONHandler(io.Discard, nil)), r.set) })
	if err != nil {
		t.Fatalf("openResources: %v", err)
	}
	r.opened = r.set.all()
	return r
}

func TestOpenAddsPendingAndSchedules(t *testing.T) {
	fake := resetFake(t)
	approvals := fakeApprovals{}
	r := openWith(t, fakeKind, testDSN, approvals, nil)
	if fake.opened != 1 || fake.openedSQL != 0 {
		t.Fatalf("Open calls = %d, OpenSQL calls = %d; want 1, 0", fake.opened, fake.openedSQL)
	}
	if fake.configs[0].Name != "main" || fake.configs[0].Approvals != approvals {
		t.Fatalf("resource config = %#v; want name main and the approval store", fake.configs[0])
	}
	if fake.creds[0] != testDSN {
		t.Fatal("adapter did not get the connection secret")
	}
	if len(r.opened) != 1 || r.opened[0] != fake.resource {
		t.Fatalf("opened = %#v", r.opened)
	}
	if len(r.sched.added) != 1 {
		t.Fatalf("scheduled = %#v; want one resource", r.sched.added)
	}
	got := r.sched.added[0]
	if got.name != "main" || got.res != fake.resource || got.timeout != time.Second+preflightSlack {
		t.Fatalf("scheduled = %#v; want main, the opened resource, %s", got, time.Second+preflightSlack)
	}
	_, err := r.inv.Lookup("skeleton", "main")
	var ce *core.Error
	if !errors.As(err, &ce) || ce.Code != core.CodeResourceUnavailable {
		t.Fatalf("Lookup before preflight = %v; want resource_unavailable", err)
	}
	if err := r.inv.SetStatus("main", inventory.StatusServed); err != nil {
		t.Fatal(err)
	}
	e, err := r.inv.Lookup("skeleton", "main")
	if err != nil {
		t.Fatal(err)
	}
	if e.Resource != fake.resource || e.Kind != fakeKind || len(e.Capabilities) != 1 || e.Capabilities[0] != "sql" || e.Limits.Timeout <= 0 {
		t.Fatalf("entry = %#v", e)
	}
	if e.Limits.IdleTimeout != e.Limits.Timeout+5*time.Second || !slices.Contains(e.Redact, "*password*") {
		t.Fatalf("entry limits = %#v, redact = %q; want the idle timeout and the default patterns", e.Limits, e.Redact)
	}
	closeResources(slog.New(slog.NewJSONHandler(io.Discard, nil)), r.set)
	if !fake.resource.closed {
		t.Fatal("closeResources did not close the resource")
	}
}

func TestOpenWrapsSQLOpener(t *testing.T) {
	fake := resetFake(t)
	var wrapped sql.Dialect
	wrap := func(d sql.Dialect) sql.Dialect {
		wrapped = d
		return d
	}
	r := openWith(t, fakeKind, testDSN, fakeApprovals{}, wrap)
	if fake.opened != 0 || fake.openedSQL != 1 {
		t.Fatalf("Open calls = %d, OpenSQL calls = %d; want 0, 1", fake.opened, fake.openedSQL)
	}
	if wrapped != fake.dialect {
		t.Fatalf("wrap got %#v; want the adapter dialect", wrapped)
	}
	if len(r.sched.added) != 1 || r.sched.added[0].res != r.opened[0] {
		t.Fatalf("scheduled = %#v; want the executor", r.sched.added)
	}
	if err := r.opened[0].Close(); err != nil || !fake.pool.closed {
		t.Fatal("executor Close did not close the pool")
	}
}

func TestOpenFailureLogsCause(t *testing.T) {
	r := openWith(t, "postgres", testDSN+"\n", fakeApprovals{}, nil)
	if !strings.Contains(r.log.String(), `"msg":"resource open failed"`) || !strings.Contains(r.log.String(), "connection secret is not a postgres URL") {
		t.Fatalf("log lacks the open cause:\n%s", r.log.String())
	}
	if strings.Contains(r.log.String(), testDSNPassword) || strings.Contains(r.log.String(), "db.example.net") {
		t.Fatal("log holds the DSN")
	}
	if len(r.opened) != 0 || len(r.sched.added) != 0 {
		t.Fatalf("failed resource was opened or scheduled: %#v, %#v", r.opened, r.sched.added)
	}
	_, err := r.inv.Lookup("skeleton", "main")
	var ce *core.Error
	if !errors.As(err, &ce) || ce.Code != core.CodeResourceUnavailable {
		t.Fatalf("Lookup = %v; want resource_unavailable", err)
	}
}

func TestOpenFailureOfSQLOpener(t *testing.T) {
	fake := resetFake(t)
	fake.openErr = core.NewError(core.CodeInvalidArgument, "connection secret is not a postgres URL")
	called := false
	r := openWith(t, fakeKind, testDSN, fakeApprovals{}, func(d sql.Dialect) sql.Dialect {
		called = true
		return d
	})
	if called || len(r.opened) != 0 || len(r.sched.added) != 0 {
		t.Fatalf("wrap called %v, opened %#v, scheduled %#v; want none", called, r.opened, r.sched.added)
	}
	if !strings.Contains(r.log.String(), `"msg":"resource open failed"`) {
		t.Fatalf("log lacks the open failure:\n%s", r.log.String())
	}
}

func TestOpenFailureRetriesOpen(t *testing.T) {
	fake := resetFake(t)
	fake.openErr = errors.New("connection secret is not a postgres URL")
	r := openWith(t, fakeKind, testDSN, fakeApprovals{}, nil)
	if len(r.opened) != 0 || len(r.sched.added) != 0 || len(r.sched.unopened) != 1 {
		t.Fatalf("opened %d, scheduled %d, unopened %d; want 0, 0, 1", len(r.opened), len(r.sched.added), len(r.sched.unopened))
	}
	retry := r.sched.unopened[0]
	if retry.name != "main" || retry.timeout != time.Second+preflightSlack {
		t.Fatalf("unopened = %+v; want main with %s", retry, time.Second+preflightSlack)
	}
	if _, err := retry.open(context.Background()); err == nil {
		t.Fatal("retry open succeeded while Open fails")
	}
	if n := strings.Count(r.log.String(), `"msg":"resource open failed"`); n != 2 {
		t.Fatalf("log has %d open failures; want 2 with the same line:\n%s", n, r.log.String())
	}
	fake.mu.Lock()
	fake.openErr = nil
	fake.mu.Unlock()
	res, err := retry.open(context.Background())
	if err != nil || res != fake.resource {
		t.Fatalf("retry open = %#v, %v; want the opened resource", res, err)
	}
	if all := r.set.all(); len(all) != 1 || all[0] != res {
		t.Fatalf("opened set = %#v; want the reopened resource for shutdown", all)
	}
	if err := r.inv.SetStatus("main", inventory.StatusServed); err != nil {
		t.Fatal(err)
	}
	if e, err := r.inv.Lookup("skeleton", "main"); err != nil || e.Resource != res {
		t.Fatalf("Lookup = %#v, %v; want the reopened resource", e, err)
	}
}
