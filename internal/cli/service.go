package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"

	"github.com/kanthorlabs/sag/internal/deploy"
	"github.com/kanthorlabs/sag/internal/podman"
	"github.com/kanthorlabs/sag/internal/service"
	"github.com/kanthorlabs/sag/internal/stepline"
	"github.com/kanthorlabs/sag/internal/testhook"
)

var (
	serviceStartImpl = defaultServiceStart
	serviceStopImpl  = defaultServiceStop
)

func serviceStart(args []string, std IO) int {
	return serviceStartImpl(args, std)
}

func serviceStop(args []string, std IO) int {
	return serviceStopImpl(args, std)
}

func defaultServiceStart(args []string, std IO) int {
	cfg, err := buildServiceConfig()
	if err != nil {
		return fail(std, err)
	}
	out := stepline.New(std.Out)
	if err := service.Start(context.Background(), cfg, out); err != nil {
		return fail(std, err)
	}
	return ExitOK
}

func defaultServiceStop(args []string, std IO) int {
	cfg, err := buildServiceConfig()
	if err != nil {
		return fail(std, err)
	}
	out := stepline.New(std.Out)
	if err := service.Stop(context.Background(), cfg, out); err != nil {
		return fail(std, err)
	}
	return ExitOK
}

func buildServiceConfig() (service.Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return service.Config{}, fmt.Errorf("home dir: %w", err)
	}
	cfgDir := resolveCfgDir(home)
	binPath, err := os.Executable()
	if err != nil {
		return service.Config{}, fmt.Errorf("executable: %w", err)
	}
	image := testhook.Image()
	if image == "" {
		image = imageRef()
	}
	podmanPath, err := findPodmanPath()
	if err != nil {
		return service.Config{}, err
	}
	return service.Config{
		Podman:  podman.Runner{Path: podmanPath},
		CfgDir:  cfgDir,
		UID:     strconv.Itoa(os.Getuid()),
		Home:    home,
		BinPath: binPath,
		Image:   image,
	}, nil
}

func resolveCfgDir(home string) string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "sag")
	}
	return filepath.Join(home, ".config", "sag")
}

func imageRef() string {
	if deploy.ImageDigest != "" {
		return "docker.io/kanthorlabs/sag@" + deploy.ImageDigest
	}
	return "docker.io/kanthorlabs/sag:main"
}

func findPodmanPath() (string, error) {
	if p, err := exec.LookPath("podman"); err == nil {
		return p, nil
	}
	for _, p := range []string{"/opt/podman/bin/podman", "/opt/homebrew/bin/podman"} {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("podman not found in PATH, /opt/podman/bin, or /opt/homebrew/bin")
}
