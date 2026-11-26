package cli

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/kanthorlabs/sag/internal/service"
	"github.com/kanthorlabs/sag/internal/stepline"
)

func withFakeCfg(t *testing.T, cfg service.Config) {
	t.Helper()
	released := false
	oldBuild := buildCfgFn
	buildCfgFn = func() (service.Config, func(), error) {
		return cfg, func() { released = true }, nil
	}
	t.Cleanup(func() {
		buildCfgFn = oldBuild
		if !released {
			t.Error("buildCfgFn release was not called")
		}
	})
}

func withFakeCfgErr(t *testing.T, err error) {
	t.Helper()
	oldBuild := buildCfgFn
	buildCfgFn = func() (service.Config, func(), error) {
		return service.Config{}, noop, err
	}
	t.Cleanup(func() { buildCfgFn = oldBuild })
}

func withGOOS(t *testing.T, goos, goarch string) {
	t.Helper()
	savedOS, savedArch := serviceGOOS, serviceGOARCH
	serviceGOOS, serviceGOARCH = goos, goarch
	t.Cleanup(func() {
		serviceGOOS, serviceGOARCH = savedOS, savedArch
	})
}

func TestServiceStartSucceeds(t *testing.T) {
	cfg := service.Config{Image: "localhost/sag:test"}
	withFakeCfg(t, cfg)

	oldStart := startFn
	var gotCfg service.Config
	startFn = func(_ context.Context, c service.Config, _ stepline.Writer) error {
		gotCfg = c
		return nil
	}
	t.Cleanup(func() { startFn = oldStart })

	std, _ := testIO()
	std.Mode = ModeHost
	if code := serviceStart(nil, std); code != ExitOK {
		t.Fatalf("serviceStart = %d; want 0", code)
	}
	if gotCfg.Image != "localhost/sag:test" {
		t.Fatalf("startFn received wrong image %q", gotCfg.Image)
	}
}

func TestServiceStartFailsOnBuildError(t *testing.T) {
	withFakeCfgErr(t, fmt.Errorf("build failed"))

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
	released := false
	oldBuild := buildCfgFn
	buildCfgFn = func() (service.Config, func(), error) {
		return service.Config{}, func() { released = true }, nil
	}
	t.Cleanup(func() { buildCfgFn = oldBuild })

	oldStart := startFn
	startFn = func(_ context.Context, _ service.Config, _ stepline.Writer) error {
		return fmt.Errorf("start failed")
	}
	t.Cleanup(func() { startFn = oldStart })

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
	withFakeCfg(t, cfg)

	oldStop := stopFn
	var gotCfg service.Config
	stopFn = func(_ context.Context, c service.Config, _ stepline.Writer) error {
		gotCfg = c
		return nil
	}
	t.Cleanup(func() { stopFn = oldStop })

	std, _ := testIO()
	std.Mode = ModeHost
	if code := serviceStop(nil, std); code != ExitOK {
		t.Fatalf("serviceStop = %d; want 0", code)
	}
	if gotCfg.Image != "localhost/sag:test" {
		t.Fatalf("stopFn received wrong image %q", gotCfg.Image)
	}
}

func TestServiceStopFailsOnBuildError(t *testing.T) {
	withFakeCfgErr(t, fmt.Errorf("build failed"))

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
	released := false
	oldBuild := buildCfgFn
	buildCfgFn = func() (service.Config, func(), error) {
		return service.Config{}, func() { released = true }, nil
	}
	t.Cleanup(func() { buildCfgFn = oldBuild })

	oldStop := stopFn
	stopFn = func(_ context.Context, _ service.Config, _ stepline.Writer) error {
		return fmt.Errorf("stop failed")
	}
	t.Cleanup(func() { stopFn = oldStop })

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

func TestServiceStartRejectsPositionalArgs(t *testing.T) {
	withFakeCfgErr(t, fmt.Errorf("should not reach build"))
	std, errOut := testIO()
	std.Mode = ModeHost
	if code := serviceStart([]string{"extra"}, std); code != ExitUsage {
		t.Fatalf("serviceStart(extra) = %d; want %d", code, ExitUsage)
	}
	if strings.Contains(errOut.String(), "should not reach build") {
		t.Fatal("buildCfgFn called despite unexpected args")
	}
}

func TestServiceStopRejectsPositionalArgs(t *testing.T) {
	withFakeCfgErr(t, fmt.Errorf("should not reach build"))
	std, errOut := testIO()
	std.Mode = ModeHost
	if code := serviceStop([]string{"extra"}, std); code != ExitUsage {
		t.Fatalf("serviceStop(extra) = %d; want %d", code, ExitUsage)
	}
	if strings.Contains(errOut.String(), "should not reach build") {
		t.Fatal("buildCfgFn called despite unexpected args")
	}
}

func TestServiceStartRejectsUnsupportedPlatform(t *testing.T) {
	withGOOS(t, "darwin", "amd64")
	withFakeCfgErr(t, fmt.Errorf("should not reach build"))
	std, errOut := testIO()
	std.Mode = ModeHost
	if code := serviceStart(nil, std); code != ExitFailure {
		t.Fatalf("serviceStart on darwin/amd64 = %d; want %d", code, ExitFailure)
	}
	if !strings.Contains(errOut.String(), "unsupported platform darwin/amd64") {
		t.Fatalf("stderr = %q; want unsupported platform", errOut.String())
	}
}

func TestServiceStopRejectsUnsupportedPlatform(t *testing.T) {
	withGOOS(t, "darwin", "amd64")
	withFakeCfgErr(t, fmt.Errorf("should not reach build"))
	std, errOut := testIO()
	std.Mode = ModeHost
	if code := serviceStop(nil, std); code != ExitFailure {
		t.Fatalf("serviceStop on darwin/amd64 = %d; want %d", code, ExitFailure)
	}
	if !strings.Contains(errOut.String(), "unsupported platform darwin/amd64") {
		t.Fatalf("stderr = %q; want unsupported platform", errOut.String())
	}
}

func TestDefaultBuildCfgLockFailReturnsError(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("HOME", dir)

	_, _, err := defaultBuildCfg()
	if err == nil {
		t.Fatal("defaultBuildCfg with absent cfg dir: expected error from AcquireLock")
	}
}
