package service

import (
	"context"

	"github.com/kanthorlabs/sag/internal/podman"
)

func checkPort(_ context.Context, _ podman.Command) error {
	return nil
}
