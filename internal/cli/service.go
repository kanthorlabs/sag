package cli

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strconv"

	"github.com/kanthorlabs/sag/internal/hoststate"
	"github.com/kanthorlabs/sag/internal/image"
	"github.com/kanthorlabs/sag/internal/podman"
	"github.com/kanthorlabs/sag/internal/service"
	"github.com/kanthorlabs/sag/internal/stepline"
)

var (
	buildCfgFn    = defaultBuildCfg
	startFn       = service.Start
	stopFn        = service.Stop
	serviceGOOS   = runtime.GOOS
	serviceGOARCH = runtime.GOARCH
)

func serviceStart(args []string, std IO) int {
	if len(args) != 0 {
		fmt.Fprintln(std.Err, "sag: service start: unexpected arguments")
		return ExitUsage
	}
	if err := service.CheckPlatform(serviceGOOS, serviceGOARCH); err != nil {
		return fail(std, err)
	}
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

func serviceStop(args []string, std IO) int {
	if len(args) != 0 {
		fmt.Fprintln(std.Err, "sag: service stop: unexpected arguments")
		return ExitUsage
	}
	if err := service.CheckPlatform(serviceGOOS, serviceGOARCH); err != nil {
		return fail(std, err)
	}
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

	imageRef, _ := image.Reference()
	cfg := service.Config{
		Podman:  podman.Runner{Path: podmanPath},
		CfgDir:  p.Cfg,
		UID:     strconv.Itoa(os.Getuid()),
		Home:    home,
		BinPath: binPath,
		Image:   imageRef,
	}
	return cfg, release, nil
}

func noop() {}
