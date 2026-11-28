package preflight

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"sync"
	"time"

	"github.com/kanthorlabs/sag/internal/core"
	"github.com/kanthorlabs/sag/internal/inventory"
)

const RetryInterval = 60 * time.Second

var closeGrace = 5 * time.Second

type StatusSetter interface {
	SetStatus(name string, s inventory.Status) error
}

type Options struct {
	Clock           Clock
	Inventory       StatusSetter
	Log             *slog.Logger
	RecheckInterval time.Duration
}

type Scheduler struct {
	opts      Options
	resources []scheduled
	loops     sync.WaitGroup
}

type scheduled struct {
	name      string
	res       core.Resource
	open      func(ctx context.Context) (core.Resource, error)
	timeout   time.Duration
	immediate bool
}

type outcome struct {
	report core.PreflightReport
	err    error
}

func New(opts Options) *Scheduler {
	return &Scheduler{opts: opts}
}

func (s *Scheduler) Add(name string, res core.Resource, timeout time.Duration) {
	s.resources = append(s.resources, scheduled{name: name, res: res, timeout: timeout})
}

func (s *Scheduler) AddUnopened(name string, open func(ctx context.Context) (core.Resource, error), timeout time.Duration) {
	s.resources = append(s.resources, scheduled{name: name, open: open, timeout: timeout})
}

func (s *Scheduler) AddUnopenedImmediate(name string, open func(ctx context.Context) (core.Resource, error), timeout time.Duration) {
	s.resources = append(s.resources, scheduled{name: name, open: open, timeout: timeout, immediate: true})
}

func (s *Scheduler) Start(ctx context.Context) {
	served := make([]bool, len(s.resources))
	var first sync.WaitGroup
	for i, r := range s.resources {
		if r.res == nil {
			continue
		}
		first.Go(func() { served[i] = s.run(ctx, r) })
	}
	first.Wait()
	n := len(s.resources)
	for i, r := range s.resources {
		if r.res == nil {
			if r.immediate {
				s.loops.Go(func() { s.openLoopImmediate(ctx, r, i, n) })
			} else {
				s.loops.Go(func() { s.openLoop(ctx, r, i, n) })
			}
			continue
		}
		s.loops.Go(func() { s.loop(ctx, r, served[i], i, n) })
	}
}

func (s *Scheduler) openLoopImmediate(ctx context.Context, r scheduled, i, n int) {
	res, err := r.open(ctx)
	if err == nil {
		r.res = res
		s.loop(ctx, r, s.run(ctx, r), n-1, n)
		return
	}
	s.openLoop(ctx, r, i, n)
}

func (s *Scheduler) openLoop(ctx context.Context, r scheduled, i, n int) {
	wait := spread(RetryInterval, i, n)
	for r.res == nil {
		select {
		case <-ctx.Done():
			return
		case <-s.opts.Clock.After(wait):
		}
		wait = RetryInterval
		res, err := r.open(ctx)
		if err != nil {
			continue
		}
		r.res = res
	}
	s.loop(ctx, r, s.run(ctx, r), n-1, n)
}

func (s *Scheduler) Wait() {
	s.loops.Wait()
}

func (s *Scheduler) loop(ctx context.Context, r scheduled, served bool, i, n int) {
	wait := spread(s.interval(served), i, n)
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.opts.Clock.After(wait):
		}
		served = s.run(ctx, r)
		wait = s.interval(served)
	}
}

func spread(d time.Duration, i, n int) time.Duration {
	if n <= 1 {
		return d
	}
	return d * time.Duration(i+1) / time.Duration(n)
}

func (s *Scheduler) interval(served bool) time.Duration {
	if served {
		return s.opts.RecheckInterval
	}
	return RetryInterval
}

func (s *Scheduler) run(ctx context.Context, r scheduled) bool {
	o := preflight(ctx, r)
	if ctx.Err() != nil {
		return false
	}
	status := s.judge(r.name, o)
	if err := s.opts.Inventory.SetStatus(r.name, status); err != nil {
		s.opts.Log.Error("resource status update failed", "resource", r.name)
		return false
	}
	return status == inventory.StatusServed
}

func preflight(ctx context.Context, r scheduled) outcome {
	runCtx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	done := make(chan outcome, 1)
	go func() {
		report, err := r.res.Preflight(runCtx)
		done <- outcome{report: report, err: err}
	}()
	select {
	case o := <-done:
		return timedOut(runCtx, o)
	case <-runCtx.Done():
	}
	grace := time.NewTimer(closeGrace)
	defer grace.Stop()
	select {
	case o := <-done:
		return timedOut(runCtx, o)
	case <-grace.C:
		return outcome{err: context.DeadlineExceeded}
	}
}

func timedOut(runCtx context.Context, o outcome) outcome {
	if o.err != nil && errors.Is(runCtx.Err(), context.DeadlineExceeded) {
		o.err = errors.Join(context.DeadlineExceeded, o.err)
	}
	return o
}

func (s *Scheduler) judge(name string, o outcome) inventory.Status {
	log := s.opts.Log
	if o.err != nil {
		log.Error("resource preflight failed", "resource", name, "cause", cause(o.err))
		return inventory.StatusUnavailable
	}
	for _, w := range o.report.Warnings {
		log.Warn("resource preflight warning", "resource", name, "warning", w)
	}
	if !o.report.Passed {
		log.Error("resource preflight refused", "resource", name, "failed_checks", failedChecks(o.report), "hint", o.report.Hint)
		return inventory.StatusUnavailable
	}
	log.Info("resource served", "resource", name)
	return inventory.StatusServed
}

func cause(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return core.PreflightTimedOut
	}
	var ce *core.Error
	if errors.As(err, &ce) {
		switch {
		case slices.Contains(core.PreflightCauses(), ce.Message):
			return ce.Message
		case ce.Code == core.CodeTimeout:
			return core.PreflightTimedOut
		case ce.Code == core.CodeResourceUnavailable:
			return core.PreflightConnectFailed
		}
	}
	return core.PreflightQueryFailed
}

func failedChecks(report core.PreflightReport) []string {
	var failed []string
	for _, c := range report.Checks {
		if !c.Passed {
			failed = append(failed, c.Name)
		}
	}
	return failed
}
