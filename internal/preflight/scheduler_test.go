package preflight

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgproto3"

	_ "github.com/kanthorlabs/sag/internal/adapter/postgres"
	"github.com/kanthorlabs/sag/internal/core"
	"github.com/kanthorlabs/sag/internal/inventory"
	"github.com/kanthorlabs/sag/internal/secret"
)

const testRecheck = 10 * time.Minute

type fakeClock struct {
	mu      sync.Mutex
	now     time.Time
	waiters []waiter
}

type waiter struct {
	at time.Time
	d  time.Duration
	ch chan time.Time
}

func newFakeClock() *fakeClock {
	return &fakeClock{now: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) After(d time.Duration) <-chan time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	ch := make(chan time.Time, 1)
	c.waiters = append(c.waiters, waiter{at: c.now.Add(d), d: d, ch: ch})
	return ch
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
	pending := c.waiters[:0]
	for _, w := range c.waiters {
		if w.at.After(c.now) {
			pending = append(pending, w)
			continue
		}
		w.ch <- c.now
	}
	c.waiters = pending
}

func (c *fakeClock) awaitWaiters(t *testing.T, n int) []time.Duration {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		c.mu.Lock()
		var ds []time.Duration
		for _, w := range c.waiters {
			ds = append(ds, w.d)
		}
		c.mu.Unlock()
		if len(ds) == n {
			slices.Sort(ds)
			return ds
		}
		if time.Now().After(deadline) {
			t.Fatalf("clock has %d waiters; want %d", len(ds), n)
		}
		time.Sleep(time.Millisecond)
	}
}

type fakeResource struct {
	mu     sync.Mutex
	calls  int
	result func(ctx context.Context) (core.PreflightReport, error)
}

func passing() func(context.Context) (core.PreflightReport, error) {
	return func(context.Context) (core.PreflightReport, error) {
		return core.PreflightReport{Passed: true, Checks: []core.PreflightCheck{{Name: "engine", Passed: true}}}, nil
	}
}

func failing(err error) func(context.Context) (core.PreflightReport, error) {
	return func(context.Context) (core.PreflightReport, error) {
		return core.PreflightReport{}, err
	}
}

func blocking() func(context.Context) (core.PreflightReport, error) {
	return func(ctx context.Context) (core.PreflightReport, error) {
		<-ctx.Done()
		return core.PreflightReport{}, ctx.Err()
	}
}

func newFake(result func(context.Context) (core.PreflightReport, error)) *fakeResource {
	return &fakeResource{result: result}
}

func (r *fakeResource) set(result func(context.Context) (core.PreflightReport, error)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.result = result
}

func (r *fakeResource) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls
}

func (r *fakeResource) Preflight(ctx context.Context) (core.PreflightReport, error) {
	r.mu.Lock()
	r.calls++
	result := r.result
	r.mu.Unlock()
	return result(ctx)
}

func (r *fakeResource) Execute(context.Context, string, string, json.RawMessage, core.Limits) (core.Result, error) {
	return core.Result{}, errors.ErrUnsupported
}

func (r *fakeResource) Close() error { return nil }

type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) lines(t *testing.T) []map[string]any {
	t.Helper()
	b.mu.Lock()
	defer b.mu.Unlock()
	var out []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(b.buf.String()), "\n") {
		if line == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("log line %q: %v", line, err)
		}
		out = append(out, m)
	}
	return out
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func (b *syncBuffer) messages(t *testing.T, resource string) []string {
	t.Helper()
	var msgs []string
	for _, m := range b.lines(t) {
		if m["resource"] == resource {
			msgs = append(msgs, m["msg"].(string))
		}
	}
	return msgs
}

type fixture struct {
	clock *fakeClock
	inv   *inventory.Inventory
	log   *syncBuffer
	sched *Scheduler
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	f := &fixture{clock: newFakeClock(), inv: inventory.New(), log: &syncBuffer{}}
	f.sched = New(Options{
		Clock:           f.clock,
		Inventory:       f.inv,
		Log:             slog.New(slog.NewJSONHandler(f.log, nil)),
		RecheckInterval: testRecheck,
	})
	return f
}

func (f *fixture) add(t *testing.T, name string, res core.Resource, timeout time.Duration) {
	t.Helper()
	err := f.inv.Add(inventory.Entry{Name: name, Kind: "postgres", Agents: []string{"alice"}, Resource: res, Status: inventory.StatusPending})
	if err != nil {
		t.Fatal(err)
	}
	f.sched.Add(name, res, timeout)
}

func (f *fixture) start(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	f.sched.Start(ctx)
	t.Cleanup(func() {
		cancel()
		f.sched.Wait()
	})
}

func (f *fixture) status(t *testing.T, name string) inventory.Status {
	t.Helper()
	_, err := f.inv.Lookup("alice", name)
	if err == nil {
		return inventory.StatusServed
	}
	var ce *core.Error
	if errors.As(err, &ce) && ce.Code == core.CodeResourceUnavailable {
		return inventory.StatusUnavailable
	}
	t.Fatalf("Lookup(%s): %v", name, err)
	return ""
}

func TestPassMarksServed(t *testing.T) {
	f := newFixture(t)
	res := newFake(passing())
	f.add(t, "main", res, time.Second)
	f.start(t)
	if got := f.status(t, "main"); got != inventory.StatusServed {
		t.Fatalf("status = %s; want served", got)
	}
	if msgs := f.log.messages(t, "main"); !slices.Equal(msgs, []string{"resource served"}) {
		t.Fatalf("log = %q; want one resource served line", msgs)
	}
}

func TestStartRunsFirstPreflightSynchronously(t *testing.T) {
	f := newFixture(t)
	slow := newFake(func(context.Context) (core.PreflightReport, error) {
		time.Sleep(50 * time.Millisecond)
		return passing()(context.Background())
	})
	f.add(t, "slow", slow, time.Second)
	f.start(t)
	if slow.count() != 1 || f.status(t, "slow") != inventory.StatusServed {
		t.Fatalf("calls = %d, status = %s after Start; want 1 and served", slow.count(), f.status(t, "slow"))
	}
}

func TestRetryFailedAfter60s(t *testing.T) {
	f := newFixture(t)
	res := newFake(failing(core.NewError(core.CodeResourceUnavailable, "preflight connect failed")))
	f.add(t, "main", res, time.Second)
	f.start(t)
	if got := f.status(t, "main"); got != inventory.StatusUnavailable {
		t.Fatalf("status = %s; want unavailable", got)
	}
	if ds := f.clock.awaitWaiters(t, 1); ds[0] != RetryInterval || RetryInterval != 60*time.Second {
		t.Fatalf("retry wait = %v; want 60s", ds)
	}
	f.clock.Advance(59 * time.Second)
	time.Sleep(20 * time.Millisecond)
	if res.count() != 1 {
		t.Fatalf("calls = %d before 60 s; want 1", res.count())
	}
	res.set(passing())
	f.clock.Advance(time.Second)
	if ds := f.clock.awaitWaiters(t, 1); ds[0] != testRecheck {
		t.Fatalf("wait after pass = %v; want recheck interval", ds)
	}
	if res.count() != 2 || f.status(t, "main") != inventory.StatusServed {
		t.Fatalf("calls = %d, status = %s; want 2 and served", res.count(), f.status(t, "main"))
	}
}

func TestRecheckServedAtInterval(t *testing.T) {
	f := newFixture(t)
	res := newFake(passing())
	f.add(t, "main", res, time.Second)
	f.start(t)
	if ds := f.clock.awaitWaiters(t, 1); ds[0] != testRecheck {
		t.Fatalf("recheck wait = %v; want %v", ds, testRecheck)
	}
	f.clock.Advance(testRecheck - time.Second)
	time.Sleep(20 * time.Millisecond)
	if res.count() != 1 {
		t.Fatalf("calls = %d before the interval; want 1", res.count())
	}
	f.clock.Advance(time.Second)
	f.clock.awaitWaiters(t, 1)
	if res.count() != 2 {
		t.Fatalf("calls = %d after the interval; want 2", res.count())
	}
	if msgs := f.log.messages(t, "main"); !slices.Equal(msgs, []string{"resource served", "resource served"}) {
		t.Fatalf("log = %q", msgs)
	}
}

func TestFailedRecheckMarksUnavailable(t *testing.T) {
	f := newFixture(t)
	res := newFake(passing())
	f.add(t, "main", res, time.Second)
	f.start(t)
	f.clock.awaitWaiters(t, 1)
	res.set(func(context.Context) (core.PreflightReport, error) {
		return core.PreflightReport{Checks: []core.PreflightCheck{{Name: "risky_objects"}}, Hint: "approve"}, nil
	})
	f.clock.Advance(testRecheck)
	if ds := f.clock.awaitWaiters(t, 1); ds[0] != RetryInterval {
		t.Fatalf("wait after failed recheck = %v; want 60s", ds)
	}
	if got := f.status(t, "main"); got != inventory.StatusUnavailable {
		t.Fatalf("status = %s; want unavailable", got)
	}
	res.set(passing())
	f.clock.Advance(RetryInterval)
	f.clock.awaitWaiters(t, 1)
	if got := f.status(t, "main"); got != inventory.StatusServed {
		t.Fatalf("status = %s after a passing check; want served", got)
	}
}

func TestStatusVisibleInInventory(t *testing.T) {
	f := newFixture(t)
	res := newFake(passing())
	f.add(t, "main", res, time.Second)
	if got := f.inv.Visible("alice"); len(got) != 0 {
		t.Fatalf("pending resource is visible before Start")
	}
	f.start(t)
	if got := f.inv.Visible("alice"); len(got) != 1 || got[0].Name != "main" {
		t.Fatalf("Visible(alice) = %v; want [main]", got)
	}
	f.clock.awaitWaiters(t, 1)
	res.set(failing(core.NewError(core.CodeResourceUnavailable, "preflight query failed")))
	f.clock.Advance(testRecheck)
	f.clock.awaitWaiters(t, 1)
	if got := f.inv.Visible("alice"); len(got) != 0 {
		t.Fatalf("Visible(alice) = %v after a failed recheck; want none", got)
	}
}

func TestRunLogsWarningsThenOneResult(t *testing.T) {
	f := newFixture(t)
	f.add(t, "skip", newFake(func(context.Context) (core.PreflightReport, error) {
		return core.PreflightReport{Passed: true, Warnings: []string{"tls verification is off"}}, nil
	}), time.Second)
	f.add(t, "ext", newFake(func(context.Context) (core.PreflightReport, error) {
		return core.PreflightReport{
			Checks:   []core.PreflightCheck{{Name: "engine", Passed: true}, {Name: "risky_objects"}, {Name: "version"}},
			Warnings: []string{"w1", "w2"},
			Hint:     "resource ext has risky objects dblink",
		}, nil
	}), time.Second)
	f.add(t, "down", newFake(failing(core.NewError(core.CodeResourceUnavailable, "preflight connect failed"))), time.Second)
	f.start(t)

	cases := map[string][]string{
		"skip": {"resource preflight warning", "resource served"},
		"ext":  {"resource preflight warning", "resource preflight warning", "resource preflight refused"},
		"down": {"resource preflight failed"},
	}
	for name, want := range cases {
		if got := f.log.messages(t, name); !slices.Equal(got, want) {
			t.Fatalf("%s log = %q; want %q", name, got, want)
		}
	}
	for _, m := range f.log.lines(t) {
		if m["msg"] == "resource preflight refused" {
			checks, _ := json.Marshal(m["failed_checks"])
			if string(checks) != `["risky_objects","version"]` || m["hint"] != "resource ext has risky objects dblink" {
				t.Fatalf("refused line = %v", m)
			}
		}
		if m["msg"] == "resource preflight failed" && m["cause"] != "preflight connect failed" {
			t.Fatalf("failed line = %v", m)
		}
	}
}

func TestFailureCauseIsFixedClass(t *testing.T) {
	const driverText = "FATAL: password authentication failed for user secret-user"
	cases := []struct {
		err  error
		want string
	}{
		{&core.Error{Code: core.CodeResourceUnavailable, Message: "preflight connect failed", Cause: errors.New(driverText)}, "preflight connect failed"},
		{&core.Error{Code: core.CodeResourceUnavailable, Message: "preflight TLS handshake failed", Cause: errors.New(driverText)}, "preflight TLS handshake failed"},
		{&core.Error{Code: core.CodeResourceUnavailable, Message: "preflight query failed", Cause: errors.New(driverText)}, "preflight query failed"},
		{&core.Error{Code: core.CodeResourceUnavailable, Message: "preflight timed out", Cause: errors.New(driverText)}, "preflight timed out"},
		{&core.Error{Code: core.CodeResourceUnavailable, Message: driverText}, "preflight connect failed"},
		{&core.Error{Code: core.CodeTimeout, Message: driverText}, "preflight timed out"},
		{fmt.Errorf("query: %w", context.DeadlineExceeded), "preflight timed out"},
		{errors.New(driverText), "preflight query failed"},
	}
	for _, c := range cases {
		f := newFixture(t)
		f.add(t, "main", newFake(failing(c.err)), time.Second)
		f.start(t)
		out := f.log.String()
		if !strings.Contains(out, `"cause":"`+c.want+`"`) {
			t.Fatalf("error %v: log %s; want cause %q", c.err, out, c.want)
		}
		if strings.Contains(out, "secret-user") {
			t.Fatalf("log holds driver text: %s", out)
		}
	}
}

func TestResourcesRunIndependently(t *testing.T) {
	f := newFixture(t)
	slowA := newFake(blocking())
	slowB := newFake(blocking())
	fast := newFake(passing())
	f.add(t, "slow-a", slowA, 400*time.Millisecond)
	f.add(t, "slow-b", slowB, 400*time.Millisecond)
	f.add(t, "fast", fast, time.Second)
	start := time.Now()
	f.start(t)
	if elapsed := time.Since(start); elapsed > 750*time.Millisecond {
		t.Fatalf("Start took %s; want first runs in parallel", elapsed)
	}
	if f.status(t, "fast") != inventory.StatusServed || f.status(t, "slow-a") != inventory.StatusUnavailable {
		t.Fatalf("status fast=%s slow-a=%s", f.status(t, "fast"), f.status(t, "slow-a"))
	}
	f.clock.awaitWaiters(t, 3)
	f.clock.Advance(RetryInterval)
	f.clock.awaitWaiters(t, 1)
	begin := time.Now()
	f.clock.Advance(testRecheck)
	deadline := time.Now().Add(5 * time.Second)
	for fast.count() < 2 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if elapsed := time.Since(begin); elapsed > 300*time.Millisecond || fast.count() != 2 {
		t.Fatalf("fast recheck after %s, calls = %d; want no wait for the slow runs", elapsed, fast.count())
	}
}

func TestRunWaitIsBounded(t *testing.T) {
	saved := closeGrace
	closeGrace = 100 * time.Millisecond
	t.Cleanup(func() { closeGrace = saved })

	f := newFixture(t)
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	f.add(t, "stuck", newFake(func(context.Context) (core.PreflightReport, error) {
		<-release
		return core.PreflightReport{Passed: true}, nil
	}), 100*time.Millisecond)
	start := time.Now()
	f.start(t)
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("Start took %s; want the run deadline plus the close grace", elapsed)
	}
	if got := f.status(t, "stuck"); got != inventory.StatusUnavailable {
		t.Fatalf("status = %s; want unavailable", got)
	}
	if !strings.Contains(f.log.String(), `"cause":"preflight timed out"`) {
		t.Fatalf("log lacks the timeout cause: %s", f.log.String())
	}
}

func TestStopOnCancel(t *testing.T) {
	f := newFixture(t)
	res := newFake(passing())
	f.add(t, "main", res, time.Second)
	ctx, cancel := context.WithCancel(context.Background())
	f.sched.Start(ctx)
	f.clock.awaitWaiters(t, 1)
	cancel()
	done := make(chan struct{})
	go func() {
		f.sched.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Wait did not return after cancel")
	}
}

func TestSystemClock(t *testing.T) {
	c := SystemClock()
	if c == nil {
		t.Fatal("SystemClock() = nil")
	}
	if d := time.Since(c.Now()); d < 0 || d > time.Minute {
		t.Fatalf("Now() differs from time.Now by %s", d)
	}
	select {
	case <-c.After(time.Millisecond):
	case <-time.After(5 * time.Second):
		t.Fatal("After did not fire")
	}
}

const stallPassword = "stall-pass-71c3"

func stalledServer(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go stall(c)
		}
	}()
	return ln.Addr().String()
}

func stall(c net.Conn) {
	defer c.Close()
	be := pgproto3.NewBackend(c, c)
	if _, err := be.ReceiveStartupMessage(); err != nil {
		return
	}
	be.Send(&pgproto3.AuthenticationOk{})
	be.Send(&pgproto3.BackendKeyData{ProcessID: 1, SecretKey: []byte{0, 0, 0, 1}})
	be.Send(&pgproto3.ReadyForQuery{TxStatus: 'I'})
	if err := be.Flush(); err != nil {
		return
	}
	io.Copy(io.Discard, c)
}

func TestPreflightStalledServerTimesOut(t *testing.T) {
	adapter, ok := core.Lookup("postgres")
	if !ok {
		t.Fatal("postgres adapter is not registered")
	}
	dsn := "postgres://u:" + stallPassword + "@" + stalledServer(t) + "/app"
	cfg := core.ResourceConfig{Name: "main", Kind: "postgres", TLS: core.TLSVerifyFull, PoolSize: 1, ConnectTimeout: time.Second}
	res, err := adapter.Open(context.Background(), cfg, secret.NewCredential("sag-main", []byte(dsn)))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { res.Close() })

	f := newFixture(t)
	f.add(t, "main", res, cfg.ConnectTimeout+500*time.Millisecond)
	f.add(t, "other", newFake(passing()), time.Second)
	start := time.Now()
	f.start(t)
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Fatalf("Start took %s; want the preflight deadline", elapsed)
	}
	if got := f.status(t, "main"); got != inventory.StatusUnavailable {
		t.Fatalf("status = %s; want unavailable", got)
	}
	if got := f.status(t, "other"); got != inventory.StatusServed {
		t.Fatalf("other status = %s; want served", got)
	}
	out := f.log.String()
	if !strings.Contains(out, `"cause":"preflight timed out"`) {
		t.Fatalf("log lacks the timeout cause:\n%s", out)
	}
	if strings.Contains(out, stallPassword) {
		t.Fatal("log holds the DSN password")
	}
}

func TestCauseKeepsEachSharedCause(t *testing.T) {
	for _, c := range core.PreflightCauses() {
		if got := cause(&core.Error{Code: core.CodeResourceUnavailable, Message: c}); got != c {
			t.Errorf("cause(%q) = %q; want the same shared cause", c, got)
		}
	}
}

func TestUnopenedResourceRetriesOpen(t *testing.T) {
	f := newFixture(t)
	if err := f.inv.Add(inventory.Entry{Name: "late", Kind: "postgres", Agents: []string{"alice"}, Status: inventory.StatusUnavailable}); err != nil {
		t.Fatal(err)
	}
	res := newFake(passing())
	var mu sync.Mutex
	opens := 0
	f.sched.AddUnopened("late", func(context.Context) (core.Resource, error) {
		mu.Lock()
		defer mu.Unlock()
		opens++
		if opens == 1 {
			return nil, errors.New("open failed")
		}
		if err := f.inv.SetResource("late", res); err != nil {
			return nil, err
		}
		return res, nil
	}, time.Second)
	f.start(t)
	if res.count() != 0 || f.status(t, "late") != inventory.StatusUnavailable {
		t.Fatal("unopened resource ran preflight or is served before a retry")
	}
	if got := f.clock.awaitWaiters(t, 1); !slices.Equal(got, []time.Duration{RetryInterval}) {
		t.Fatalf("waiters = %v; want one open retry at %s", got, RetryInterval)
	}
	f.clock.Advance(RetryInterval)
	if got := f.clock.awaitWaiters(t, 1); !slices.Equal(got, []time.Duration{RetryInterval}) {
		t.Fatalf("waiters after a failed open = %v; want %s", got, RetryInterval)
	}
	if res.count() != 0 {
		t.Fatal("preflight ran after a failed open")
	}
	f.clock.Advance(RetryInterval)
	if got := f.clock.awaitWaiters(t, 1); !slices.Equal(got, []time.Duration{testRecheck}) {
		t.Fatalf("waiters after a successful open = %v; want the recheck %s", got, testRecheck)
	}
	if res.count() != 1 || f.status(t, "late") != inventory.StatusServed {
		t.Fatalf("preflight runs %d, status %s; want one run and served", res.count(), f.status(t, "late"))
	}
}

func TestFirstRechecksSpreadOverOneInterval(t *testing.T) {
	f := newFixture(t)
	for _, name := range []string{"a", "b", "c", "d"} {
		f.add(t, name, newFake(passing()), time.Second)
	}
	f.start(t)
	q := testRecheck / 4
	if got := f.clock.awaitWaiters(t, 4); !slices.Equal(got, []time.Duration{q, 2 * q, 3 * q, testRecheck}) {
		t.Fatalf("first recheck waits = %v; want %v %v %v %v", got, q, 2*q, 3*q, testRecheck)
	}
	f.clock.Advance(q)
	deadline := time.Now().Add(5 * time.Second)
	for {
		got := f.clock.awaitWaiters(t, 4)
		if slices.Equal(got, []time.Duration{2 * q, 3 * q, testRecheck, testRecheck}) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("waits after the first recheck = %v; want the full interval for the next recheck", got)
		}
		time.Sleep(time.Millisecond)
	}
}

func TestFirstRetriesSpreadOverRetryInterval(t *testing.T) {
	f := newFixture(t)
	if err := f.inv.Add(inventory.Entry{Name: "late", Kind: "postgres", Agents: []string{"alice"}, Status: inventory.StatusUnavailable}); err != nil {
		t.Fatal(err)
	}
	f.sched.AddUnopened("late", func(context.Context) (core.Resource, error) {
		return nil, errors.New("open failed")
	}, time.Second)
	f.add(t, "a", newFake(failing(core.NewError(core.CodeResourceUnavailable, "preflight connect failed"))), time.Second)
	f.add(t, "b", newFake(failing(core.NewError(core.CodeResourceUnavailable, "preflight connect failed"))), time.Second)
	f.start(t)
	third := RetryInterval / 3
	if got := f.clock.awaitWaiters(t, 3); !slices.Equal(got, []time.Duration{third, 2 * third, RetryInterval}) {
		t.Fatalf("first retry waits = %v; want %v %v %v", got, third, 2*third, RetryInterval)
	}
	f.clock.Advance(third)
	deadline := time.Now().Add(5 * time.Second)
	for {
		got := f.clock.awaitWaiters(t, 3)
		if slices.Equal(got, []time.Duration{2 * third, RetryInterval, RetryInterval}) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("waits after a failed open = %v; want the full retry interval for the next open", got)
		}
		time.Sleep(time.Millisecond)
	}
}
