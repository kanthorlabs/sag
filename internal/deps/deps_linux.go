//go:build linux

package deps

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

func DefaultPodmanVersion(ctx context.Context, path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("deps: podman path is empty")
	}
	out, err := exec.CommandContext(ctx, path, "--version").Output()
	if err != nil {
		return "", fmt.Errorf("deps: podman --version: %w", err)
	}
	line := strings.TrimSpace(string(out))
	parts := strings.Fields(line)
	if len(parts) < 3 {
		return "", fmt.Errorf("deps: unexpected podman --version output: %q", line)
	}
	return parts[2], nil
}

var DefaultMachineList func(context.Context, string) ([]MachineInfo, error) = nil
var DefaultMachineInit func(context.Context, string) error = nil
var DefaultMachineStart func(context.Context, string) error = nil
