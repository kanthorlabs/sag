//go:build linux

package portcheck

import "context"

func Check(_ context.Context, _ string) (Holder, error) {
	return Holder{}, nil
}
