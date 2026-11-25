package pipeline

import (
	"github.com/kanthorlabs/sag/internal/core"
	"github.com/kanthorlabs/sag/internal/inventory"
)

func resolveLimits(entry inventory.Entry, _ Request) (core.Limits, error) {
	return entry.Limits, nil
}
