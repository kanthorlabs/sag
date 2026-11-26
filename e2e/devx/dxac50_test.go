//go:build devxe2e

package devx_test

import (
	"context"
	"testing"
)

func DXAC50_TailscaleFake(t *testing.T, _ context.Context) {
	t.Helper()
	t.Skip("D3.2 pending")
}
