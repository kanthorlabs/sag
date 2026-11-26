//go:build darwin

package deps

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
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

func DefaultDownload(ctx context.Context, url, dir string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("deps: download %s: HTTP %d", url, resp.StatusCode)
	}
	name := filepath.Base(url)
	dst := filepath.Join(dir, name)
	f, err := os.Create(dst)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if _, err := io.Copy(f, resp.Body); err != nil {
		return "", err
	}
	return dst, nil
}

func DefaultSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
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
