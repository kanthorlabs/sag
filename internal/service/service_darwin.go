//go:build darwin

package service

import (
	"context"
	"errors"
)

func Start(ctx context.Context, cfg Config) error {
	return errors.New("not implemented")
}

func Stop(ctx context.Context, cfg Config) error {
	return errors.New("not implemented")
}
