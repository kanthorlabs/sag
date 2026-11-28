package dxh

import (
	"context"
	"fmt"
)

func UserJournal(ctx context.Context, r Runner, n int) ([]byte, error) {
	return r.Output(ctx, "journalctl", "--user", "-u", "sag", "-n", fmt.Sprintf("%d", n))
}
