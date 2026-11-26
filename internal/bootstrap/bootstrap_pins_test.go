//go:build devxtest

package bootstrap

import (
	"context"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/kanthorlabs/sag/internal/stepline"
)

func runSelectPins(t *testing.T, goos, goarch string) (*State, error) {
	t.Helper()
	d := Deps{
		GOOS:   func() string { return goos },
		GOARCH: func() string { return goarch },
		OpenFile: func(path string) error {
			return os.ErrNotExist
		},
		UID:     func() string { return "501" },
		Home:    func() string { return t.TempDir() },
		SAGPath: func() (string, error) { return "/usr/local/bin/sag", nil },
		Out:     stepline.New(io.Discard),
		Print:   io.Discard,
	}
	steps := d.BuildSteps()
	s := &State{Out: d.Out}
	if err := steps[0].Run(context.Background(), s); err != nil {
		t.Fatalf("platform-check: %v", err)
	}
	return s, steps[1].Run(context.Background(), s)
}

func TestMalformedPinLineFailsBootstrap(t *testing.T) {
	t.Setenv("SAG_TEST_PINS", "podman 5.5.1 darwin/arm64 release https://example.com/p.pkg\n")

	_, err := runSelectPins(t, "darwin", "arm64")
	if err == nil {
		t.Fatal("select-pins with malformed line: want error, got nil")
	}
	if !strings.Contains(err.Error(), "pins") {
		t.Fatalf("error = %q; want 'pins' in message", err.Error())
	}
}

func TestZeroPinsForPlatformFails(t *testing.T) {
	t.Setenv("SAG_TEST_PINS", "podman 5.5.1 linux/* apt podman -\n")

	_, err := runSelectPins(t, "darwin", "arm64")
	if err == nil {
		t.Fatal("select-pins with no matching platform: want error, got nil")
	}
	if !strings.Contains(err.Error(), "darwin") {
		t.Fatalf("error = %q; want platform name", err.Error())
	}
}
