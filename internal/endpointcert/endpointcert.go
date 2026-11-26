package endpointcert

import (
	"context"
	"errors"
)

type Config struct {
	PodmanPath  string
	CfgDir      string
	TailscaleIP string
}

func Create(ctx context.Context, cfg Config) error {
	return errors.New("not implemented")
}

func Renew(ctx context.Context, cfg Config) error {
	return errors.New("not implemented")
}
