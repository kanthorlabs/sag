package deps

import (
	"context"

	"github.com/kanthorlabs/sag/internal/pins"
)

type Plan struct {
	Items []PlanItem
}

type PlanItem struct {
	Pin     pins.Item
	Absent  bool
	Older   bool
	Newer   bool
	Approve bool
}

func Make(_ context.Context, _ []pins.Item) (Plan, error) {
	return Plan{}, nil
}

func Apply(_ context.Context, _ Plan) error {
	return nil
}
