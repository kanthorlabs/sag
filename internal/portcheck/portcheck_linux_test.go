//go:build linux

package portcheck

import (
	"context"
	"testing"
)

func TestCheckLinuxFreePort(t *testing.T) {
	h, err := Check(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if h.Kind != HolderNone {
		t.Fatalf("Kind = %v; want HolderNone", h.Kind)
	}
}
