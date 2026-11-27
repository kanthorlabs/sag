package cli

import (
	"context"
	"testing"

	"github.com/kanthorlabs/sag/internal/daemon"
	"github.com/kanthorlabs/sag/internal/sql"
)

func TestServeRejectsArgs(t *testing.T) {
	std, _ := testIO()
	if code := Run([]string{"serve", "extra"}, std); code != ExitUsage {
		t.Fatalf("Run(serve extra) = %d; want %d", code, ExitUsage)
	}
}

func captureDaemonOptions(t *testing.T) *daemon.Options {
	t.Helper()
	saved := runDaemon
	t.Cleanup(func() { runDaemon = saved })
	var got daemon.Options
	called := false
	runDaemon = func(_ context.Context, opts daemon.Options) error {
		got, called = opts, true
		return nil
	}
	t.Cleanup(func() {
		if !called {
			t.Error("daemon.Run was not called")
		}
	})
	return &got
}

func TestServeProductHasNoWrapDialect(t *testing.T) {
	got := captureDaemonOptions(t)
	std, _ := testIO()
	if code := Run([]string{"serve"}, std); code != ExitOK {
		t.Fatalf("Run(serve) = %d; want %d", code, ExitOK)
	}
	if got.WrapDialect != nil {
		t.Fatal("Run(serve) passes a WrapDialect to daemon.Run; the product path must pass nil")
	}
}

func TestServeWithWrapPassesWrapDialect(t *testing.T) {
	got := captureDaemonOptions(t)
	std, _ := testIO()
	if code := RunWith([]string{"serve"}, std, func(d sql.Dialect) sql.Dialect { return d }); code != ExitOK {
		t.Fatalf("RunWith(serve) = %d; want %d", code, ExitOK)
	}
	if got.WrapDialect == nil {
		t.Fatal("RunWith(serve, wrap) passes no WrapDialect; the seam does not see the option")
	}
}
