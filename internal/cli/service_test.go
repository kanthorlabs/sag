package cli

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/kanthorlabs/sag/internal/service"
	"github.com/kanthorlabs/sag/internal/stepline"
)

func withFakeCfg(cfg service.Config) (restore func()) {
	released := false
	oldBuild := buildCfgFn
	buildCfgFn = func() (service.Config, func(), error) {
		return cfg, func() { released = true }, nil
	}
	return func() {
		buildCfgFn = oldBuild
		if !released {
			panic("buildCfgFn release was not called")
		}
	}
}

func withFakeCfgErr(err error) (restore func()) {
	oldBuild := buildCfgFn
	buildCfgFn = func() (service.Config, func(), error) {
		return service.Config{}, noop, err
	}
	return func() { buildCfgFn = oldBuild }
}

func TestServiceStartSucceeds(t *testing.T) {
	cfg := service.Config{Image: "localhost/sag:test"}
	restore := withFakeCfg(cfg)

	oldStart := startFn
	var gotCfg service.Config
	startFn = func(_ context.Context, c service.Config, _ stepline.Writer) error {
		gotCfg = c
		return nil
	}
	defer func() { startFn = oldStart }()

	std, _ := testIO()
	std.Mode = ModeHost
	if code := serviceStart(nil, std); code != ExitOK {
		t.Fatalf("serviceStart = %d; want 0", code)
	}
	restore()
	if gotCfg.Image != "localhost/sag:test" {
		t.Fatalf("startFn received wrong image %q", gotCfg.Image)
	}
}

func TestServiceStartFailsOnBuildError(t *testing.T) {
	restore := withFakeCfgErr(fmt.Errorf("build failed"))
	defer restore()

	std, errOut := testIO()
	std.Mode = ModeHost
	if code := serviceStart(nil, std); code != ExitFailure {
		t.Fatalf("serviceStart = %d; want %d", code, ExitFailure)
	}
	if !strings.Contains(errOut.String(), "build failed") {
		t.Fatalf("stderr = %q; want 'build failed'", errOut.String())
	}
}

func TestServiceStartReleasesOnError(t *testing.T) {
	cfg := service.Config{}
	released := false
	oldBuild := buildCfgFn
	buildCfgFn = func() (service.Config, func(), error) {
		return cfg, func() { released = true }, nil
	}
	defer func() { buildCfgFn = oldBuild }()

	oldStart := startFn
	startFn = func(_ context.Context, _ service.Config, _ stepline.Writer) error {
		return fmt.Errorf("start failed")
	}
	defer func() { startFn = oldStart }()

	std, _ := testIO()
	std.Mode = ModeHost
	code := serviceStart(nil, std)
	if code != ExitFailure {
		t.Fatalf("serviceStart = %d; want %d", code, ExitFailure)
	}
	if !released {
		t.Fatal("release not called on start error")
	}
}

func TestServiceStopSucceeds(t *testing.T) {
	cfg := service.Config{Image: "localhost/sag:test"}
	restore := withFakeCfg(cfg)

	oldStop := stopFn
	var gotCfg service.Config
	stopFn = func(_ context.Context, c service.Config, _ stepline.Writer) error {
		gotCfg = c
		return nil
	}
	defer func() { stopFn = oldStop }()

	std, _ := testIO()
	std.Mode = ModeHost
	if code := serviceStop(nil, std); code != ExitOK {
		t.Fatalf("serviceStop = %d; want 0", code)
	}
	restore()
	if gotCfg.Image != "localhost/sag:test" {
		t.Fatalf("stopFn received wrong image %q", gotCfg.Image)
	}
}

func TestServiceStopFailsOnBuildError(t *testing.T) {
	restore := withFakeCfgErr(fmt.Errorf("build failed"))
	defer restore()

	std, errOut := testIO()
	std.Mode = ModeHost
	if code := serviceStop(nil, std); code != ExitFailure {
		t.Fatalf("serviceStop = %d; want %d", code, ExitFailure)
	}
	if !strings.Contains(errOut.String(), "build failed") {
		t.Fatalf("stderr = %q; want 'build failed'", errOut.String())
	}
}

func TestServiceStopReleasesOnError(t *testing.T) {
	cfg := service.Config{}
	released := false
	oldBuild := buildCfgFn
	buildCfgFn = func() (service.Config, func(), error) {
		return cfg, func() { released = true }, nil
	}
	defer func() { buildCfgFn = oldBuild }()

	oldStop := stopFn
	stopFn = func(_ context.Context, _ service.Config, _ stepline.Writer) error {
		return fmt.Errorf("stop failed")
	}
	defer func() { stopFn = oldStop }()

	std, _ := testIO()
	std.Mode = ModeHost
	code := serviceStop(nil, std)
	if code != ExitFailure {
		t.Fatalf("serviceStop = %d; want %d", code, ExitFailure)
	}
	if !released {
		t.Fatal("release not called on stop error")
	}
}
