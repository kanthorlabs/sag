package pipeline

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/kanthorlabs/sag/internal/core"
	"github.com/kanthorlabs/sag/internal/inventory"
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
	return core.Result{}, errors.ErrUnsupported
}
