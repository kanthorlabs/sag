package daemon

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"time"

	"github.com/kanthorlabs/sag/internal/config"
	"github.com/kanthorlabs/sag/internal/core"
	"github.com/kanthorlabs/sag/internal/inventory"
	"github.com/kanthorlabs/sag/internal/secret"
)

var preflightSlack = 10 * time.Second

func openResources(ctx context.Context, log *slog.Logger, cfg *config.Config, creds map[string]secret.Credential, inv *inventory.Inventory) ([]core.Resource, error) {
	var opened []core.Resource
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
			Status:       inventory.StatusPending,
		}
		res, err := adapter.Open(ctx, ResourceConfig(name, r, creds, nil), creds[string(r.Connection)])
		if err != nil {
			log.Error("resource open failed", "resource", name, "error", err.Error())
			entry.Status = inventory.StatusUnavailable
			if err := inv.Add(entry); err != nil {
				return opened, fmt.Errorf("%s: %w", name, err)
			}
			continue
		}
		opened = append(opened, res)
		entry.Resource = res
		if err := inv.Add(entry); err != nil {
			return opened, fmt.Errorf("%s: %w", name, err)
		}
		if err := inv.SetStatus(name, preflight(ctx, log, name, res, r.Limits.ConnectTimeout+preflightSlack)); err != nil {
			return opened, fmt.Errorf("%s: %w", name, err)
		}
	}
	return opened, nil
}

func preflight(ctx context.Context, log *slog.Logger, name string, res core.Resource, timeout time.Duration) inventory.Status {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	report, err := res.Preflight(ctx)
	if err != nil {
		log.Error("resource preflight failed", "resource", name, "cause", preflightCause(err))
		return inventory.StatusUnavailable
	}
	for _, w := range report.Warnings {
		log.Warn("resource preflight warning", "resource", name, "warning", w)
	}
	if !report.Passed {
		log.Error("resource preflight refused", "resource", name, "failed_checks", failedChecks(report), "hint", report.Hint)
		return inventory.StatusUnavailable
	}
	log.Info("resource served", "resource", name)
	return inventory.StatusServed
}

func preflightCause(err error) string {
	var coreErr *core.Error
	if errors.As(err, &coreErr) {
		return coreErr.Message
	}
	return "unclassified"
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

func capabilityNames(caps []core.Capability) []string {
	names := make([]string, 0, len(caps))
	for _, c := range caps {
		names = append(names, c.Name)
	}
	return names
}

func closeResources(log *slog.Logger, opened []core.Resource) {
	for _, res := range opened {
		if err := res.Close(); err != nil {
			log.Error("resource close failed")
		}
	}
}
