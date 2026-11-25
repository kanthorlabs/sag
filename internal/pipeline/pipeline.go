package pipeline

import (
	"context"
	"encoding/json"
	"errors"
	"slices"

	"github.com/kanthorlabs/sag/internal/core"
	"github.com/kanthorlabs/sag/internal/inventory"
)

const (
	messageNotFound     = "resource not found"
	messageUnavailable  = "resource unavailable"
	messageNoCapability = "resource has no such capability"
)

type Resolver interface {
	Lookup(sub, name string) (inventory.Entry, error)
}

type Request struct {
	ID         string
	Sub        string
	Resource   string
	Capability string
	Operation  string
	Input      json.RawMessage
}

type Options struct {
	Inventory Resolver
}

type Pipeline struct {
	opts Options
}

func New(opts Options) *Pipeline {
	return &Pipeline{opts: opts}
}

func (p *Pipeline) Execute(ctx context.Context, req Request) (core.Result, error) {
	entry, err := p.authorize(req)
	if err != nil {
		return core.Result{}, err
	}
	return entry.Resource.Execute(ctx, req.Capability, req.Operation, req.Input, entry.Limits)
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
