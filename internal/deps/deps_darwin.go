//go:build darwin

package deps

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

var DefaultAptCandidateVersion func(ctx context.Context, name string) (string, error) = nil
var DefaultAptInstall func(ctx context.Context, name string, noInteractive bool) error = nil
var DefaultFindNewuidmap func() bool = nil

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

func DefaultMachineList(ctx context.Context, path string) ([]MachineInfo, error) {
	out, err := exec.CommandContext(ctx, path, "machine", "list", "--format", "{{.Name}} {{.Running}}").Output()
	if err != nil {
		return nil, fmt.Errorf("deps: machine list: %w", err)
	}
	var machines []MachineInfo
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		machines = append(machines, MachineInfo{
			Name:    parts[0],
			Running: parts[1] == "true",
		})
	}
	return machines, nil
}

func DefaultMachineInit(ctx context.Context, path string) error {
	return exec.CommandContext(ctx, path, "machine", "init").Run()
}

func DefaultMachineStart(ctx context.Context, path string) error {
	return exec.CommandContext(ctx, path, "machine", "start").Run()
}
