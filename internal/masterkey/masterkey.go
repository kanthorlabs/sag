package masterkey

import (
	"context"
	"errors"

	"github.com/kanthorlabs/sag/internal/podman"
	"github.com/kanthorlabs/sag/internal/stepline"
)

func Ensure(ctx context.Context, pm podman.Command, out stepline.Writer) (changed bool, err error) {
	return false, errors.New("not implemented")
}
