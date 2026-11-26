package service

import (
	"context"

	"github.com/kanthorlabs/sag/internal/podman"
	"github.com/kanthorlabs/sag/internal/stepline"
)

func checkPort(_ context.Context, _ podman.Command, _ stepline.Writer) error {
	return nil
}
