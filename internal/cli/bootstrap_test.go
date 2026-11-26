package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/kanthorlabs/sag/internal/bootstrap"
	"github.com/kanthorlabs/sag/internal/deps"
	"github.com/kanthorlabs/sag/internal/prompt"
)

func bootstrapTestIO() (IO, *bytes.Buffer) {
	errOut := &bytes.Buffer{}
	return IO{
		In:         strings.NewReader(""),
		Out:        io.Discard,
		Err:        errOut,
		IsTerminal: func(any) bool { return false },
		Mode:       ModeHost,
	}, errOut
}

func TestBootstrapFlags(t *testing.T) {
	saved := runBootstrap
	defer func() { runBootstrap = saved }()

	var capturedFlags bootstrap.Flags
	runBootstrap = func(ctx context.Context, flags bootstrap.Flags, std IO) error {
		capturedFlags = flags
		return nil
	}

	t.Run("unknown flag gives exit 2", func(t *testing.T) {
		std, _ := bootstrapTestIO()
		code := bootstrapCmd([]string{"--no-such-flag"}, std)
		if code != ExitUsage {
			t.Fatalf("bootstrapCmd(--no-such-flag) = %d; want %d", code, ExitUsage)
		}
	})

	t.Run("bad autostart value gives exit 2", func(t *testing.T) {
		std, _ := bootstrapTestIO()
		code := bootstrapCmd([]string{"--autostart", "maybe"}, std)
		if code != ExitUsage {
			t.Fatalf("bootstrapCmd(--autostart maybe) = %d; want %d", code, ExitUsage)
		}
	})

	t.Run("bad agent-setup value gives exit 2", func(t *testing.T) {
		std, _ := bootstrapTestIO()
		code := bootstrapCmd([]string{"--agent-setup", "claude-code=bad"}, std)
		if code != ExitUsage {
			t.Fatalf("bootstrapCmd(--agent-setup claude-code=bad) = %d; want %d", code, ExitUsage)
		}
	})

	t.Run("refusal error gives exit 3", func(t *testing.T) {
		runBootstrap = func(ctx context.Context, flags bootstrap.Flags, std IO) error {
			return &prompt.RefusalError{Flag: "--autostart"}
		}
		defer func() { runBootstrap = saved }()
		std, errOut := bootstrapTestIO()
		code := bootstrapCmd(nil, std)
		if code != ExitRefusal {
			t.Fatalf("bootstrapCmd() with RefusalError = %d; want %d", code, ExitRefusal)
		}
		if !strings.Contains(errOut.String(), "--autostart") {
			t.Fatalf("stderr = %q; want --autostart", errOut.String())
		}
	})

	t.Run("other error gives exit 1", func(t *testing.T) {
		runBootstrap = func(ctx context.Context, flags bootstrap.Flags, std IO) error {
			return errors.New("something failed")
		}
		defer func() { runBootstrap = saved }()
		std, _ := bootstrapTestIO()
		code := bootstrapCmd(nil, std)
		if code != ExitFailure {
			t.Fatalf("bootstrapCmd() with error = %d; want %d", code, ExitFailure)
		}
	})

	t.Run("InstallRefusal gives exit 3", func(t *testing.T) {
		runBootstrap = func(ctx context.Context, flags bootstrap.Flags, std IO) error {
			return &deps.InstallRefusal{Name: "testpkg", Version: "1.0.0"}
		}
		defer func() { runBootstrap = saved }()
		std, errOut := bootstrapTestIO()
		code := bootstrapCmd(nil, std)
		if code != ExitRefusal {
			t.Fatalf("bootstrapCmd() with InstallRefusal = %d; want %d", code, ExitRefusal)
		}
		if !strings.Contains(errOut.String(), "testpkg") {
			t.Fatalf("stderr = %q; want testpkg in message", errOut.String())
		}
	})

	t.Run("SudoRefusal gives exit 3", func(t *testing.T) {
		runBootstrap = func(ctx context.Context, flags bootstrap.Flags, std IO) error {
			return &deps.SudoRefusal{}
		}
		defer func() { runBootstrap = saved }()
		std, errOut := bootstrapTestIO()
		code := bootstrapCmd(nil, std)
		if code != ExitRefusal {
			t.Fatalf("bootstrapCmd() with SudoRefusal = %d; want %d", code, ExitRefusal)
		}
		if !strings.Contains(errOut.String(), "sudo") {
			t.Fatalf("stderr = %q; want sudo in message", errOut.String())
		}
	})

	t.Run("OlderError gives exit 1", func(t *testing.T) {
		runBootstrap = func(ctx context.Context, flags bootstrap.Flags, std IO) error {
			return &deps.OlderError{Name: "testpkg", Installed: "4.0.0", Pin: "5.0.0"}
		}
		defer func() { runBootstrap = saved }()
		std, _ := bootstrapTestIO()
		code := bootstrapCmd(nil, std)
		if code != ExitFailure {
			t.Fatalf("bootstrapCmd() with OlderError = %d; want %d", code, ExitFailure)
		}
	})

	runBootstrap = func(ctx context.Context, flags bootstrap.Flags, std IO) error {
		capturedFlags = flags
		return nil
	}

	t.Run("agent-setup claude-code=no is parsed", func(t *testing.T) {
		std, _ := bootstrapTestIO()
		code := bootstrapCmd([]string{"--agent-setup", "claude-code=no"}, std)
		if code != ExitOK {
			t.Fatalf("bootstrapCmd(--agent-setup claude-code=no) = %d; want %d", code, ExitOK)
		}
		if capturedFlags.AgentSetup["claude-code"] != "no" {
			t.Fatalf("AgentSetup[claude-code] = %q; want %q", capturedFlags.AgentSetup["claude-code"], "no")
		}
	})

	t.Run("autostart yes is parsed", func(t *testing.T) {
		std, _ := bootstrapTestIO()
		code := bootstrapCmd([]string{"--autostart", "yes"}, std)
		if code != ExitOK {
			t.Fatalf("bootstrapCmd(--autostart yes) = %d; want %d", code, ExitOK)
		}
		if capturedFlags.Autostart != "yes" {
			t.Fatalf("Flags.Autostart = %q; want %q", capturedFlags.Autostart, "yes")
		}
	})

	t.Run("success gives exit 0", func(t *testing.T) {
		std, _ := bootstrapTestIO()
		code := bootstrapCmd(nil, std)
		if code != ExitOK {
			t.Fatalf("bootstrapCmd() = %d; want %d", code, ExitOK)
		}
	})

	t.Run("positional argument gives exit 2", func(t *testing.T) {
		std, _ := bootstrapTestIO()
		code := bootstrapCmd([]string{"extra"}, std)
		if code != ExitUsage {
			t.Fatalf("bootstrapCmd(extra) = %d; want %d", code, ExitUsage)
		}
	})
}
