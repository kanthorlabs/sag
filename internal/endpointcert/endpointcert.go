package endpointcert

import (
	"context"
	"errors"
	"time"

	"github.com/kanthorlabs/sag/internal/podman"
	"github.com/kanthorlabs/sag/internal/stepline"
)

type Config struct {
	CfgDir      string
	TailscaleIP string
}

func Ensure(ctx context.Context, pm podman.Command, cfg Config, now time.Time, out stepline.Writer) (changed bool, err error) {
	return false, errors.New("not implemented")
}
