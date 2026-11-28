//go:build linux

package tailscale

import (
	"context"
	"errors"

	"github.com/kanthorlabs/sag/internal/stepline"
)

func EnsureOperator(_ context.Context, _ Command, _ string, _ bool, _ func() (bool, error), _ stepline.Writer) error {
	return errors.New("tailscale: operator step not implemented")
}
