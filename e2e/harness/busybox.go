package harness

import (
	"context"
	"errors"
)

var errBusyboxNotImplemented = errors.New("harness: BusyboxMount is not implemented")

func BusyboxMount(ctx context.Context, dir string) (string, error) {
	return "", errBusyboxNotImplemented
}
