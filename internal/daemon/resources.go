package daemon

import (
	"context"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"sync"
	"time"

	"github.com/kanthorlabs/sag/internal/config"
	"github.com/kanthorlabs/sag/internal/core"
	"github.com/kanthorlabs/sag/internal/inventory"
	"github.com/kanthorlabs/sag/internal/scrub"
	"github.com/kanthorlabs/sag/internal/secret"
	"github.com/kanthorlabs/sag/internal/sql"
)

var preflightSlack = 10 * time.Second

type resourceScheduler interface {
	Add(name string, res core.Resource, timeout time.Duration)
	AddUnopened(name string, open func(ctx context.Context) (core.Resource, error), timeout time.Duration)
	AddUnopenedImmediate(name string, open func(ctx context.Context) (core.Resource, error), timeout time.Duration)
}

type resourceSet struct {
	mu   sync.Mutex
	list []core.Resource
}

func (s *resourceSet) add(res core.Resource) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.list = append(s.list, res)
}

func (s *resourceSet) all() []core.Resource {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.list)
}

func openResources(ctx context.Context, log *slog.Logger, cfg *config.Config, creds map[string]secret.Credential, loader AWSLoader, scrubber scrub.Scrubber, inv *inventory.Inventory, approvals core.ApprovalVerifier, wrap func(sql.Dialect) sql.Dialect, sched resourceScheduler) (*resourceSet, error) {
	opened := &resourceSet{}
	for _, name := range slices.Sorted(maps.Keys(cfg.Resources)) {
		r := cfg.Resources[name]
		adapter, ok := core.Lookup(r.Kind)
		if !ok {
			return opened, fmt.Errorf("%s: unknown kind %q", name, r.Kind)
		}
		entry := inventory.Entry{
			Name:         name,
			Kind:         r.Kind,
			Description:  r.Description,
			Capabilities: capabilityNames(adapter.Capabilities()),
			Agents:       r.Agents,
			Limits:       resourceLimits(r.Limits),
			Redact:       r.RedactColumns,
			Status:       inventory.StatusPending,
		}
		rc := ResourceConfig(name, r, creds, approvals)
		timeout := r.Limits.ConnectTimeout + preflightSlack
		if loader != nil && len(AWSRefs(r)) > 0 {
			entry.Status = inventory.StatusUnavailable
			if err := inv.Add(entry); err != nil {
				return opened, fmt.Errorf("%s: %w", name, err)
			}
			openFn := awsOpenFunc(log, loader, scrubber, inv, opened, name, r, creds, adapter, approvals, wrap)
			sched.AddUnopenedImmediate(name, openFn, timeout)
			continue
		}
		cred := creds[string(r.Connection)]
		res, err := openResource(ctx, adapter, rc, cred, wrap)
		if err != nil {
			log.Error("resource open failed", "resource", name, "error", err.Error())
			entry.Status = inventory.StatusUnavailable
			if err := inv.Add(entry); err != nil {
				return opened, fmt.Errorf("%s: %w", name, err)
			}
			sched.AddUnopened(name, func(ctx context.Context) (core.Resource, error) {
				return reopen(ctx, log, inv, opened, name, adapter, rc, cred, wrap)
			}, timeout)
			continue
		}
		opened.add(res)
		entry.Resource = res
		if err := inv.Add(entry); err != nil {
			return opened, fmt.Errorf("%s: %w", name, err)
		}
		sched.Add(name, res, timeout)
	}
	return opened, nil
}

func reopen(ctx context.Context, log *slog.Logger, inv *inventory.Inventory, opened *resourceSet, name string, adapter core.Adapter, rc core.ResourceConfig, cred secret.Credential, wrap func(sql.Dialect) sql.Dialect) (core.Resource, error) {
	res, err := openResource(ctx, adapter, rc, cred, wrap)
	if err != nil {
		log.Error("resource open failed", "resource", name, "error", err.Error())
		return nil, err
	}
	if err := inv.SetResource(name, res); err != nil {
		_ = res.Close()
		return nil, err
	}
	opened.add(res)
	return res, nil
}

func openResource(ctx context.Context, adapter core.Adapter, rc core.ResourceConfig, cred secret.Credential, wrap func(sql.Dialect) sql.Dialect) (core.Resource, error) {
	if opener, ok := adapter.(sql.Opener); ok && wrap != nil {
		d, p, err := opener.OpenSQL(ctx, rc, cred)
		if err != nil {
			return nil, err
		}
		return sql.NewExecutor(wrap(d), p), nil
	}
	return adapter.Open(ctx, rc, cred)
}

func capabilityNames(caps []core.Capability) []string {
	names := make([]string, 0, len(caps))
	for _, c := range caps {
		names = append(names, c.Name)
	}
	return names
}

func closeResources(log *slog.Logger, opened *resourceSet) {
	for _, res := range opened.all() {
		if err := res.Close(); err != nil {
			log.Error("resource close failed")
		}
	}
}
