package dxh_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kanthorlabs/sag/e2e/devx/dxh"
)

func TestVersionGate(t *testing.T) {
	ctx := context.Background()

	t.Run("correct version passes", func(t *testing.T) {
		r := newFake()
		r.set("claude --version", []byte("Claude Code 2.1.332\n"), nil)
		if err := dxh.VersionGate(ctx, r, "claude", "2.1.332"); err != nil {
			t.Errorf("VersionGate: correct version: got error %v; want nil", err)
		}
	})

	t.Run("wrong version prerequisite missing", func(t *testing.T) {
		r := newFake()
		r.set("claude --version", []byte("Claude Code 2.0.0\n"), nil)
		err := dxh.VersionGate(ctx, r, "claude", "2.1.332")
		if err == nil {
			t.Fatal("VersionGate: wrong version: got nil; want error")
		}
		if !strings.Contains(err.Error(), "prerequisite missing") {
			t.Errorf("VersionGate: error = %q; want 'prerequisite missing'", err.Error())
		}
	})

	t.Run("command error prerequisite missing", func(t *testing.T) {
		r := newFake()
		r.set("pi --version", nil, errors.New("not found"))
		err := dxh.VersionGate(ctx, r, "pi", "1.6.1")
		if err == nil {
			t.Fatal("VersionGate: command error: got nil; want error")
		}
		if !strings.Contains(err.Error(), "prerequisite missing") {
			t.Errorf("VersionGate: error = %q; want 'prerequisite missing'", err.Error())
		}
	})

	t.Run("pi correct version passes", func(t *testing.T) {
		r := newFake()
		r.set("pi --version", []byte("pi 1.6.1\n"), nil)
		if err := dxh.VersionGate(ctx, r, "pi", "1.6.1"); err != nil {
			t.Errorf("VersionGate: correct pi version: got error %v; want nil", err)
		}
	})
}

func TestOutputHasNoJWT(t *testing.T) {
	jwt := "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiJjbGF1ZGUtY29kZSIsIm5hbWUiOiJjbGF1ZGUtY29kZSJ9.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c"

	t.Run("jwt removed", func(t *testing.T) {
		out := dxh.FilterNoJWT("some output " + jwt + " more output")
		if strings.Contains(out, jwt) {
			t.Errorf("FilterNoJWT: JWT still present in output: %q", out)
		}
		if !strings.Contains(out, "some output") || !strings.Contains(out, "more output") {
			t.Errorf("FilterNoJWT: non-JWT content removed: %q", out)
		}
	})

	t.Run("non-jwt content preserved", func(t *testing.T) {
		input := "sag: connected\nresource: x\nok agent claude-code\n"
		out := dxh.FilterNoJWT(input)
		if out != input {
			t.Errorf("FilterNoJWT: non-JWT content changed: got %q; want %q", out, input)
		}
	})

	t.Run("jwt replaced with placeholder", func(t *testing.T) {
		out := dxh.FilterNoJWT(jwt)
		if out != "<JWT>" {
			t.Errorf("FilterNoJWT: JWT not replaced with <JWT>: got %q", out)
		}
	})

	t.Run("multiple jwts removed", func(t *testing.T) {
		two := jwt + " and " + jwt
		out := dxh.FilterNoJWT(two)
		if strings.Contains(out, jwt) {
			t.Errorf("FilterNoJWT: JWT still present after double replacement: %q", out)
		}
	})
}
