package dxh

import (
	"context"
	"runtime"
	"strings"
)

type Env struct {
	OS         string
	Podman     string
	ClaudeCode string
	Pi         string
	Tailscale  string
}

func Record(ctx context.Context) (Env, error) {
	return RecordWith(ctx, NewRunner())
}

func RecordWith(ctx context.Context, r Runner) (Env, error) {
	var e Env
	var err error

	e.OS, err = recordOS(ctx, r)
	if err != nil {
		return Env{}, err
	}
	e.Podman, err = recordVersion(ctx, r, "podman", "--version")
	if err != nil {
		return Env{}, err
	}
	e.ClaudeCode, err = recordVersion(ctx, r, "claude", "--version")
	if err != nil {
		return Env{}, err
	}
	e.Pi, err = recordVersion(ctx, r, "pi", "--version")
	if err != nil {
		return Env{}, err
	}
	e.Tailscale, err = recordVersion(ctx, r, "tailscale", "version")
	if err != nil {
		return Env{}, err
	}
	return e, nil
}

func recordOS(ctx context.Context, r Runner) (string, error) {
	switch runtime.GOOS {
	case "darwin":
		out, err := r.Output(ctx, "sw_vers", "-productVersion")
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(out)), nil
	default:
		return runtime.GOOS, nil
	}
}

func recordVersion(ctx context.Context, r Runner, name string, args ...string) (string, error) {
	out, err := r.Output(ctx, name, args...)
	if err != nil {
		return "", err
	}
	line := strings.SplitN(strings.TrimSpace(string(out)), "\n", 2)[0]
	return strings.TrimSpace(line), nil
}
