package pipeline

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/kanthorlabs/sag/internal/core"
	"github.com/kanthorlabs/sag/internal/inventory"
)

func TestFifthRequestRateLimited(t *testing.T) {
	s := newSemaphore(4)
	var releases []func()
	for i := range 4 {
		release, err := s.acquire("ac23")
		if err != nil {
			t.Fatalf("acquire %d: %v", i+1, err)
		}
		releases = append(releases, release)
	}
	_, err := s.acquire("ac23")
	if ce := assertCode(t, err, core.CodeRateLimited); ce.Message == "" {
		t.Fatal("rate_limited error has no message")
	}
	if release, err := s.acquire("other"); err != nil {
		t.Fatalf("other sub: %v", err)
	} else {
		release()
	}
	releases[0]()
	releases[0]()
	release, err := s.acquire("ac23")
	if err != nil {
		t.Fatalf("acquire after release: %v", err)
	}
	release()
	if _, err := s.acquire("ac23"); err != nil {
		t.Fatalf("acquire after second release: %v", err)
	}
	if _, err := s.acquire("ac23"); err == nil {
		t.Fatal("double release freed two slots")
	}
}

func TestUnlimitedSemaphore(t *testing.T) {
	s := newSemaphore(0)
	for range 100 {
		if _, err := s.acquire("ac23"); err != nil {
			t.Fatal(err)
		}
	}
}

func TestConcurrentAcquireHoldsLimit(t *testing.T) {
	s := newSemaphore(4)
	var wg sync.WaitGroup
	var mu sync.Mutex
	granted := 0
	for range 50 {
		wg.Go(func() {
			if _, err := s.acquire("ac23"); err == nil {
				mu.Lock()
				granted++
				mu.Unlock()
			}
		})
	}
	wg.Wait()
	if granted != 4 {
		t.Fatalf("granted = %d; want 4", granted)
	}
}

type blockingResource struct {
	entered chan struct{}
	proceed chan struct{}
	err     error
}

func (b *blockingResource) Preflight(context.Context) (core.PreflightReport, error) {
	return core.PreflightReport{Passed: true}, nil
}

func (b *blockingResource) Execute(context.Context, string, string, json.RawMessage, core.Limits) (core.Result, error) {
	b.entered <- struct{}{}
	<-b.proceed
	return core.Result{}, b.err
}

func (b *blockingResource) Close() error { return nil }

type panicResource struct{}

func (panicResource) Preflight(context.Context) (core.PreflightReport, error) {
	return core.PreflightReport{Passed: true}, nil
}

func (panicResource) Execute(context.Context, string, string, json.RawMessage, core.Limits) (core.Result, error) {
	panic("driver panic")
}

func (panicResource) Close() error { return nil }

func TestSharedAcrossResources(t *testing.T) {
	a := &blockingResource{entered: make(chan struct{}, 4), proceed: make(chan struct{})}
	b := &blockingResource{entered: make(chan struct{}, 4), proceed: make(chan struct{})}
	p := New(Options{
		Inventory: realInventory(t,
			entry("rate-a", inventory.StatusServed, a, "ac23"),
			entry("rate-b", inventory.StatusServed, b, "ac23"),
		),
		MaxConcurrentPerAgent: 4,
	})
	release := sync.OnceFunc(func() {
		close(a.proceed)
		close(b.proceed)
	})
	t.Cleanup(release)
	var wg sync.WaitGroup
	errs := make(chan error, 4)
	for _, name := range []string{"rate-a", "rate-a", "rate-b", "rate-b"} {
		wg.Go(func() {
			_, err := p.Execute(context.Background(), request("ac23", name))
			errs <- err
		})
	}
	timeout := time.After(5 * time.Second)
	for held := 0; held < 4; {
		select {
		case <-a.entered:
			held++
		case <-b.entered:
			held++
		case err := <-errs:
			t.Fatalf("held request returned before release: %v", err)
		case <-timeout:
			t.Fatalf("%d of 4 held requests entered Execute", held)
		}
	}
	for _, name := range []string{"rate-a", "rate-b"} {
		over := make(chan error, 1)
		go func() {
			_, err := p.Execute(context.Background(), request("ac23", name))
			over <- err
		}()
		select {
		case err := <-over:
			assertCode(t, err, core.CodeRateLimited)
		case <-time.After(5 * time.Second):
			t.Fatalf("request over the limit on %s did not return", name)
		}
	}
	release()
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("held request: %v", err)
		}
	}
}

func TestReleaseOnError(t *testing.T) {
	failing := &fakeResource{err: core.NewError(core.CodeUpstreamError, "upstream error")}
	p := New(Options{
		Inventory:             realInventory(t, entry("main", inventory.StatusServed, failing, "ac23")),
		MaxConcurrentPerAgent: 1,
	})
	for range 3 {
		_, err := p.Execute(context.Background(), request("ac23", "main"))
		assertCode(t, err, core.CodeUpstreamError)
	}
	p = New(Options{
		Inventory:             realInventory(t, entry("main", inventory.StatusServed, panicResource{}, "ac23")),
		MaxConcurrentPerAgent: 1,
	})
	for range 2 {
		func() {
			defer func() {
				if recover() == nil {
					t.Fatal("no panic")
				}
			}()
			_, _ = p.Execute(context.Background(), request("ac23", "main"))
		}()
	}
	if len(failing.calls) != 3 {
		t.Fatalf("Execute calls = %d; want 3", len(failing.calls))
	}
}
