//go:build darwin

package portcheck

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"strings"
)

type portMap struct {
	HostPort uint16 `json:"host_port"`
}

type portEntry struct {
	Names []string  `json:"Names"`
	Ports []portMap `json:"Ports"`
}

func parsePodmanPS(data []byte) (Holder, error) {
	var entries []portEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return Holder{}, err
	}
	for _, e := range entries {
		for _, p := range e.Ports {
			if p.HostPort != 8443 {
				continue
			}
			for _, n := range e.Names {
				if n == "sag" {
					return Holder{Kind: HolderSAG, Name: n}, nil
				}
			}
			if len(e.Names) > 0 {
				return Holder{Kind: HolderContainer, Name: e.Names[0]}, nil
			}
			return Holder{Kind: HolderContainer}, nil
		}
	}
	return Holder{}, nil
}

func parseLsof(data []byte) (Holder, error) {
	for i, line := range bytes.Split(bytes.TrimSpace(data), []byte("\n")) {
		if i == 0 {
			continue
		}
		fields := strings.Fields(string(line))
		if len(fields) == 0 {
			continue
		}
		return Holder{Kind: HolderProcess, Name: fields[0]}, nil
	}
	return Holder{}, nil
}

func check(ctx context.Context, podmanPS, lsof func(context.Context) ([]byte, error)) (Holder, error) {
	if podmanPS != nil {
		if psOut, psErr := podmanPS(ctx); psErr == nil {
			if h, parseErr := parsePodmanPS(psOut); parseErr == nil && h.Kind != HolderNone {
				return h, nil
			}
		}
	}
	lsofOut, err := lsof(ctx)
	if err != nil {
		return Holder{}, err
	}
	if len(lsofOut) == 0 {
		return Holder{}, nil
	}
	return parseLsof(lsofOut)
}

func realPodmanPS(podmanPath string) func(context.Context) ([]byte, error) {
	return func(ctx context.Context) ([]byte, error) {
		return exec.CommandContext(ctx, podmanPath, "ps", "--format", "json").Output()
	}
}

func realLsof(ctx context.Context) ([]byte, error) {
	out, err := exec.CommandContext(ctx, "lsof", "-nP", "-iTCP:8443", "-sTCP:LISTEN").Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			return nil, nil
		}
		return nil, err
	}
	return out, nil
}

func Check(ctx context.Context, podmanPath string) (Holder, error) {
	var psFunc func(context.Context) ([]byte, error)
	if podmanPath != "" {
		psFunc = realPodmanPS(podmanPath)
	}
	return check(ctx, psFunc, realLsof)
}
