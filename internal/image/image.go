package image

import (
	"context"
	"errors"

	"github.com/kanthorlabs/sag/internal/podman"
	"github.com/kanthorlabs/sag/internal/stepline"
)

func Reference() string {
	return ""
}

func Ensure(ctx context.Context, pm podman.Command, ref string, out stepline.Writer) (changed bool, err error) {
	return false, errors.New("not implemented")
}
