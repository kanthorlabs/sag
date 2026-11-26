//go:build linux

package deps

import (
	"context"
	"errors"
)

func DefaultPodmanVersion(_ context.Context, _ string) (string, error) {
	return "", errors.New("deps: unsupported platform linux")
}

func DefaultDownload(_ context.Context, _, _ string) (string, error) {
	return "", errors.New("deps: unsupported platform linux")
}

func DefaultSHA256(_ string) (string, error) {
	return "", errors.New("deps: unsupported platform linux")
}

func DefaultMachineList(_ context.Context, _ string) ([]MachineInfo, error) {
	return nil, errors.New("deps: unsupported platform linux")
}

func DefaultMachineInit(_ context.Context, _ string) error {
	return errors.New("deps: unsupported platform linux")
}

func DefaultMachineStart(_ context.Context, _ string) error {
	return errors.New("deps: unsupported platform linux")
}
