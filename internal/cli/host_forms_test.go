package cli

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kanthorlabs/sag/internal/podman"
)

func hostFormsIO(terminalIn, terminalOut bool) (IO, *bytes.Buffer, *bytes.Buffer) {
	out, errOut := &bytes.Buffer{}, &bytes.Buffer{}
	std := IO{In: strings.NewReader(""), Out: out, Err: errOut, Mode: ModeHost}
	std.IsTerminal = func(stream any) bool {
		return (terminalIn && stream == std.In) || (terminalOut && stream == std.Out)
	}
	return std, out, errOut
}

func installFakeHostPodman(t *testing.T, path string) {
	t.Helper()
	saved := loadHostPodmanPath
	t.Cleanup(func() { loadHostPodmanPath = saved })
	loadHostPodmanPath = func() (string, error) { return path, nil }
}

func TestHostFormArgs(t *testing.T) {
	installFakeHostPodman(t, "/fake/podman")

	t.Run("jwt_generate_tty", func(t *testing.T) {
		var gotArgs []string
		savedTTY := execHostTTY
		t.Cleanup(func() { execHostTTY = savedTTY })
		execHostTTY = func(_ context.Context, _ string, args []string) error {
			gotArgs = args
			return nil
		}
		std, _, _ := hostFormsIO(true, true)
		if code := hostJWTGenerate([]string{"sub", "--name", "My Sub"}, std); code != ExitOK {
			t.Fatalf("exit = %d; want 0", code)
		}
		want := []string{"sag", "sag", "jwt", "generate", "sub", "--name", "My Sub"}
		if !equalStringSlice(gotArgs, want) {
			t.Errorf("args = %v; want %v", gotArgs, want)
		}
	})

	t.Run("extension_approve_tty", func(t *testing.T) {
		var gotArgs []string
		savedTTY := execHostTTY
		t.Cleanup(func() { execHostTTY = savedTTY })
		execHostTTY = func(_ context.Context, _ string, args []string) error {
			gotArgs = args
			return nil
		}
		std, _, _ := hostFormsIO(true, true)
		if code := hostExtensionApprove([]string{"myres"}, std); code != ExitOK {
			t.Fatalf("exit = %d; want 0", code)
		}
		want := []string{"sag", "sag", "extension", "approve", "myres"}
		if !equalStringSlice(gotArgs, want) {
			t.Errorf("args = %v; want %v", gotArgs, want)
		}
	})

	t.Run("extension_revoke_forward", func(t *testing.T) {
		var gotArgs []string
		savedFwd := execHostForward
		t.Cleanup(func() { execHostForward = savedFwd })
		execHostForward = func(_ context.Context, _ string, _ IO, args []string) int {
			gotArgs = args
			return ExitOK
		}
		std, _, _ := hostFormsIO(false, false)
		if code := hostExtensionRevoke([]string{"myres"}, std); code != ExitOK {
			t.Fatalf("exit = %d; want 0", code)
		}
		want := []string{"exec", "sag", "sag", "extension", "revoke", "myres"}
		if !equalStringSlice(gotArgs, want) {
			t.Errorf("args = %v; want %v", gotArgs, want)
		}
	})

	t.Run("resource_test_forward", func(t *testing.T) {
		var gotArgs []string
		savedFwd := execHostForward
		t.Cleanup(func() { execHostForward = savedFwd })
		execHostForward = func(_ context.Context, _ string, _ IO, args []string) int {
			gotArgs = args
			return ExitOK
		}
		std, _, _ := hostFormsIO(false, false)
		if code := hostResourceTest([]string{"myres"}, std); code != ExitOK {
			t.Fatalf("exit = %d; want 0", code)
		}
		want := []string{"exec", "sag", "sag", "resource", "test", "myres"}
		if !equalStringSlice(gotArgs, want) {
			t.Errorf("args = %v; want %v", gotArgs, want)
		}
	})
}

func TestRedirectExit3(t *testing.T) {
	installFakeHostPodman(t, "/fake/podman")
	savedTTY := execHostTTY
	t.Cleanup(func() { execHostTTY = savedTTY })
	execHostTTY = func(_ context.Context, _ string, _ []string) error {
		return nil
	}

	for _, tc := range []struct{ in, out bool }{{false, true}, {true, false}, {false, false}} {
		std, _, errOut := hostFormsIO(tc.in, tc.out)
		if code := hostJWTGenerate([]string{"sub"}, std); code != ExitRefusal {
			t.Fatalf("in=%v out=%v: exit = %d; want %d", tc.in, tc.out, code, ExitRefusal)
		}
		if !strings.Contains(errOut.String(), "terminal") {
			t.Fatalf("in=%v out=%v: stderr = %q; want a terminal message", tc.in, tc.out, errOut.String())
		}
	}

	for _, tc := range []struct{ in, out bool }{{false, true}, {true, false}, {false, false}} {
		std, _, errOut := hostFormsIO(tc.in, tc.out)
		if code := hostExtensionApprove([]string{"res"}, std); code != ExitRefusal {
			t.Fatalf("approve in=%v out=%v: exit = %d; want %d", tc.in, tc.out, code, ExitRefusal)
		}
		if !strings.Contains(errOut.String(), "terminal") {
			t.Fatalf("approve in=%v out=%v: stderr = %q; want a terminal message", tc.in, tc.out, errOut.String())
		}
	}
}

func TestJSONExit2(t *testing.T) {
	installFakeHostPodman(t, "/fake/podman")
	savedTTY := execHostTTY
	t.Cleanup(func() { execHostTTY = savedTTY })
	execHostTTY = func(_ context.Context, _ string, _ []string) error {
		return errors.New("should not be called")
	}

	for _, args := range [][]string{
		{"--json"},
		{"sub", "--json"},
		{"--json", "sub"},
	} {
		std, _, _ := hostFormsIO(false, false)
		if code := hostJWTGenerate(args, std); code != ExitUsage {
			t.Fatalf("args %v: exit = %d; want %d", args, code, ExitUsage)
		}
	}
}

func TestTTYExit(t *testing.T) {
	if got := ttyExit(nil); got != ExitOK {
		t.Fatalf("nil err: exit = %d; want 0", got)
	}
	for _, code := range []int{0, 1, 2, 3} {
		err := &podman.RunError{ExitCode: code}
		if got := ttyExit(err); got != code {
			t.Fatalf("RunError{%d}: exit = %d; want %d", code, got, code)
		}
	}
	for _, code := range []int{-1, 4, 127} {
		err := &podman.RunError{ExitCode: code}
		if got := ttyExit(err); got != ExitFailure {
			t.Fatalf("RunError{%d}: exit = %d; want %d (ExitFailure)", code, got, ExitFailure)
		}
	}
}

func equalStringSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
