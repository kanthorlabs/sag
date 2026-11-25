package preflight

import (
	"context"
	"log/slog"
	"time"

	"github.com/kanthorlabs/sag/internal/core"
	"github.com/kanthorlabs/sag/internal/inventory"
)

const RetryInterval = 60 * time.Second

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
	opts Options
}

func New(opts Options) *Scheduler {
	return &Scheduler{opts: opts}
}

func (s *Scheduler) Add(name string, res core.Resource, timeout time.Duration) {}

func (s *Scheduler) Start(ctx context.Context) {}

func (s *Scheduler) Wait() {}
