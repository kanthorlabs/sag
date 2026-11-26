//go:build linux

package service

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/kanthorlabs/sag/internal/stepline"
)

type nopRunner struct{}

func (nopRunner) Run(_ context.Context, _ io.Reader, _ ...string) ([]byte, error) {
	return nil, nil
}

func TestLinuxUnsupported(t *testing.T) {
	cfg := Config{Podman: nopRunner{}}
	var buf strings.Builder
	w := stepline.New(&buf)

	t.Run("Start", func(t *testing.T) {
		err := Start(context.Background(), cfg, w)
		if err == nil || !strings.Contains(err.Error(), "unsupported platform") {
			t.Fatalf("Start returned unexpected: %v", err)
		}
	})

	t.Run("Stop", func(t *testing.T) {
		err := Stop(context.Background(), cfg, w)
		if err == nil || !strings.Contains(err.Error(), "unsupported platform") {
			t.Fatalf("Stop returned unexpected: %v", err)
		}
	})

	t.Run("Running", func(t *testing.T) {
		_, err := Running(context.Background(), cfg)
		if err == nil || !strings.Contains(err.Error(), "unsupported platform") {
			t.Fatalf("Running returned unexpected: %v", err)
		}
	})

	t.Run("ApplyAutostart", func(t *testing.T) {
		_, err := ApplyAutostart(context.Background(), cfg, true, w)
		if err == nil || !strings.Contains(err.Error(), "unsupported platform") {
			t.Fatalf("ApplyAutostart returned unexpected: %v", err)
		}
	})
}
