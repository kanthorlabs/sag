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
	buildCfgFn = defaultBuildCfg
	startFn    = service.Start
	stopFn     = service.Stop
)

func serviceStart(_ []string, std IO) int {
	cfg, release, err := buildCfgFn()
	if err != nil {
		return fail(std, err)
	}
	defer release()
	out := stepline.New(std.Out)
	if err := startFn(context.Background(), cfg, out); err != nil {
		return fail(std, err)
	}
	return ExitOK
}

func serviceStop(_ []string, std IO) int {
	cfg, release, err := buildCfgFn()
	if err != nil {
		return fail(std, err)
	}
	defer release()
	out := stepline.New(std.Out)
	if err := stopFn(context.Background(), cfg, out); err != nil {
		return fail(std, err)
	}
	return ExitOK
}

func defaultBuildCfg() (service.Config, func(), error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return service.Config{}, noop, fmt.Errorf("home dir: %w", err)
	}
	cfgDir := resolveCfgDir(home)
	binPath, err := os.Executable()
	if err != nil {
		return service.Config{}, noop, fmt.Errorf("executable: %w", err)
	}
	image := testhook.Image()
	if image == "" {
		image = imageRef()
	}
	podmanPath, err := findPodmanPath()
	if err != nil {
		return service.Config{}, noop, err
	}
	cfg := service.Config{
		Podman:  podman.Runner{Path: podmanPath},
		CfgDir:  cfgDir,
		UID:     strconv.Itoa(os.Getuid()),
		Home:    home,
		BinPath: binPath,
		Image:   image,
	}
	return cfg, noop, nil
}

func noop() {}

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
