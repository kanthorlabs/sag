//go:build darwin

package service

import (
	"context"
	"errors"

	"github.com/kanthorlabs/sag/internal/stepline"
)

func Start(ctx context.Context, cfg Config, out stepline.Writer) error {
	return errors.New("not implemented")
}

func Stop(ctx context.Context, cfg Config, out stepline.Writer) error {
	return errors.New("not implemented")
}

func Running(ctx context.Context, cfg Config) (bool, error) {
	return false, errors.New("not implemented")
}

func ApplyAutostart(ctx context.Context, cfg Config, enable bool, out stepline.Writer) (changed bool, err error) {
	return false, errors.New("not implemented")
}
