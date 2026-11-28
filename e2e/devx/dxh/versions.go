package dxh

import (
	"context"
	"errors"
)

type Env struct {
	OS         string
	Podman     string
	ClaudeCode string
	Pi         string
	Tailscale  string
}

func Record(ctx context.Context) (Env, error) {
	return Env{}, errors.New("dxh: version record not implemented")
}
