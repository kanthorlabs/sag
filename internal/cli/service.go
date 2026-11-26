package cli

import (
	"context"
	"os"
	"strconv"

	"github.com/kanthorlabs/sag/internal/deploy"
	"github.com/kanthorlabs/sag/internal/hoststate"
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
	p, err := hoststate.ResolvePaths()
	if err != nil {
		return service.Config{}, noop, err
	}
	lock, err := hoststate.AcquireLock(p)
	if err != nil {
		return service.Config{}, noop, err
	}
	release := func() { _ = lock.Release() }

	answers, err := hoststate.LoadAnswers(p)
	if err != nil {
		release()
		return service.Config{}, noop, err
	}

	podmanPath, err := hoststate.FindPodman(answers.PodmanPath)
	if err != nil {
		release()
		return service.Config{}, noop, err
	}

	home, err := os.UserHomeDir()
	if err != nil {
		release()
		return service.Config{}, noop, err
	}

	binPath, err := os.Executable()
	if err != nil {
		release()
		return service.Config{}, noop, err
	}

	image := testhook.Image()
	if image == "" {
		image = imageRef()
	}

	cfg := service.Config{
		Podman:  podman.Runner{Path: podmanPath},
		CfgDir:  p.Cfg,
		UID:     strconv.Itoa(os.Getuid()),
		Home:    home,
		BinPath: binPath,
		Image:   image,
	}
	return cfg, release, nil
}

func noop() {}

func imageRef() string {
	if deploy.ImageDigest != "" {
		return "docker.io/kanthorlabs/sag@" + deploy.ImageDigest
	}
	return "docker.io/kanthorlabs/sag:main"
}
