//go:build devxe2e

package devx_test

import (
	"context"
	"testing"

	"github.com/kanthorlabs/sag/e2e/devx/dxh"
)

func DXAC03_BootstrapIdempotent(t *testing.T, _ context.Context) {
	t.Helper()
	dxh.EnsureBootstrapped(t)
	t.Skip("D3.3 pending")
}
