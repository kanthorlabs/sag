package pipeline

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"slices"
	"time"

	"github.com/kanthorlabs/sag/internal/audit"
	"github.com/kanthorlabs/sag/internal/core"
	"github.com/kanthorlabs/sag/internal/inventory"
	"github.com/kanthorlabs/sag/internal/postexec"
	"github.com/kanthorlabs/sag/internal/scrub"
)

const (
	messageNotFound     = "resource not found"
	messageUnavailable  = "resource unavailable"
	messageNoCapability = "resource has no such capability"
	messageInternal     = "internal error"
)

type Resolver interface {
	Lookup(sub, name string) (inventory.Entry, error)
}

type Request struct {
	ID         string
	Sub        string
	JTI        string
	RemoteAddr string
	Resource   string
	Capability string
	Operation  string
	Input      json.RawMessage
}

type Options struct {
	Inventory             Resolver
	MaxConcurrentPerAgent int
	Scrubber              scrub.Scrubber
	Audit                 audit.Writer
	Log                   *slog.Logger
	Now                   func() time.Time
}

type stages struct {
	resolveLimits func(entry inventory.Entry, req Request) (core.Limits, error)
	acquire       func(sub string) (release func(), err error)
	postexec      func(res core.Result, lim core.Limits, redact []string) core.Result
	scrub         func(err error) error
	audit         func(rec audit.Record) error
}

type Pipeline struct {
	opts   Options
	stages stages
}

func New(opts Options) *Pipeline {
	if opts.Log == nil {
		opts.Log = slog.New(slog.DiscardHandler)
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	appendRecord := func(audit.Record) error { return nil }
	if opts.Audit != nil {
		appendRecord = opts.Audit.Append
	}
	return &Pipeline{
		opts: opts,
		stages: stages{
			resolveLimits: resolveLimits,
			acquire:       newSemaphore(opts.MaxConcurrentPerAgent).acquire,
			postexec:      postexec.Apply,
			scrub:         opts.Scrubber.Error,
			audit:         appendRecord,
		},
	}
}

func (p *Pipeline) Execute(ctx context.Context, req Request) (core.Result, error) {
	start := p.opts.Now()
	res, err := p.execute(ctx, req)
	return p.finish(req, start, res, err)
}

func (p *Pipeline) Reject(req Request, err error) error {
	_, err = p.finish(req, p.opts.Now(), core.Result{}, err)
	return err
}

func (p *Pipeline) Listed(req Request, count int) error {
	rec := p.record(req, p.opts.Now(), core.Result{}, nil)
	rec.Rows = count
	return p.append(rec)
}

func (p *Pipeline) execute(ctx context.Context, req Request) (core.Result, error) {
	entry, err := p.authorize(req)
	if err != nil {
		return core.Result{}, err
	}
	lim, err := p.stages.resolveLimits(entry, req)
	if err != nil {
		return core.Result{}, err
	}
	release, err := p.stages.acquire(req.Sub)
	if err != nil {
		return core.Result{}, err
	}
	defer release()
	res, err := entry.Resource.Execute(ctx, req.Capability, req.Operation, req.Input, lim)
	if err != nil {
		return core.Result{}, err
	}
	return p.stages.postexec(res, lim, entry.Redact), nil
}

func (p *Pipeline) finish(req Request, start time.Time, res core.Result, err error) (core.Result, error) {
	rec := p.record(req, start, res, err)
	if err != nil {
		err = p.stages.scrub(err)
		p.logCause(req.ID, err)
	}
	if appendErr := p.append(rec); appendErr != nil {
		return core.Result{}, appendErr
	}
	if err != nil {
		return core.Result{}, err
	}
	return res, nil
}

func (p *Pipeline) append(rec audit.Record) error {
	if err := p.stages.audit(rec); err != nil {
		return &core.Error{Code: core.CodeInternal, Message: messageInternal, Cause: err}
	}
	return nil
}

func (p *Pipeline) record(req Request, start time.Time, res core.Result, err error) audit.Record {
	rec := audit.Record{
		TS:         audit.Timestamp(start),
		RequestID:  req.ID,
		RemoteAddr: req.RemoteAddr,
		Sub:        optional(req.Sub),
		JTI:        optional(req.JTI),
		Resource:   req.Resource,
		Capability: req.Capability,
		Operation:  req.Operation,
		Decision:   audit.DecisionAllow,
		DurationMS: p.opts.Now().Sub(start).Milliseconds(),
	}
	trace := res.Trace
	if err != nil {
		rec.Decision = audit.DecisionDeny
		rec.Reason = reason(err)
		var ce *core.Error
		if errors.As(err, &ce) {
			trace = ce.Trace
		}
	} else {
		rec.Rows = len(res.Rows)
		rec.Bytes = res.Bytes
		rec.Truncated = res.Truncated
	}
	rec.SQL = trace.Input
	rec.SQLSHA256 = trace.InputSHA256
	rec.RenderedSHA256 = trace.RenderedSHA256
	return rec
}

func (p *Pipeline) logCause(id string, err error) {
	var ce *core.Error
	if errors.As(err, &ce) && ce.Code == core.CodeResourceUnavailable && ce.Cause != nil {
		p.opts.Log.Warn("request resource unavailable", "request_id", id, "cause", ce.Cause.Error())
	}
}

func reason(err error) string {
	code := core.CodeInternal
	var ce *core.Error
	if errors.As(err, &ce) && ce.Code != "" {
		code = ce.Code
	}
	var r interface{ Reason() string }
	if errors.As(err, &r) {
		return string(code) + ":" + r.Reason()
	}
	return string(code)
}

func optional(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func (p *Pipeline) authorize(req Request) (inventory.Entry, error) {
	entry, err := p.opts.Inventory.Lookup(req.Sub, req.Resource)
	if err != nil {
		var ce *core.Error
		if errors.As(err, &ce) && ce.Code == core.CodeResourceUnavailable {
			return inventory.Entry{}, core.NewError(core.CodeResourceUnavailable, messageUnavailable)
		}
		return inventory.Entry{}, core.NewError(core.CodeNotFound, messageNotFound)
	}
	if req.Sub == "" || entry.Name != req.Resource || !slices.Contains(entry.Agents, req.Sub) {
		return inventory.Entry{}, core.NewError(core.CodeNotFound, messageNotFound)
	}
	if entry.Status != inventory.StatusServed || entry.Resource == nil {
		return inventory.Entry{}, core.NewError(core.CodeResourceUnavailable, messageUnavailable)
	}
	if !slices.Contains(entry.Capabilities, req.Capability) {
		return inventory.Entry{}, core.NewError(core.CodeInvalidArgument, messageNoCapability)
	}
	return entry, nil
}
