//go:build devxe2e

package devx_test

import (
	"context"
	"testing"
)

func init() {
	macTests["DXAC35_MacCredential"] = DXAC35_MacCredential
}

func DXAC35_MacCredential(t *testing.T, ctx context.Context) {
	t.Helper()
	DXAC09_CreateCanary(t, ctx)
	DXAC17_ListInspect(t, ctx)
	DXAC20_Delete(t, ctx)
}
