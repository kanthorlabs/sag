package cli

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kanthorlabs/sag/internal/token"
)

var jwtTestKey = bytes.Repeat([]byte{0x33}, 32)

type fakeIssuer struct {
	loaded    bool
	keyLoaded bool
}

func (f *fakeIssuer) install(t *testing.T, known ...string) {
	t.Helper()
	saved := loadJWTIssuer
	t.Cleanup(func() { loadJWTIssuer = saved })
	loadJWTIssuer = func() (jwtIssuer, error) {
		f.loaded = true
		return jwtIssuer{
			Lifetime: time.Hour,
			Known: func(sub string) bool {
				for _, k := range known {
					if k == sub {
						return true
					}
				}
				return false
			},
			Key: func() ([]byte, error) {
				f.keyLoaded = true
				return jwtTestKey, nil
			},
		}, nil
	}
}

func jwtIO(terminalOut bool) (IO, *bytes.Buffer, *bytes.Buffer) {
	out, errOut := &bytes.Buffer{}, &bytes.Buffer{}
	std := IO{In: strings.NewReader(""), Out: out, Err: errOut, Mode: ModeContainer}
	std.IsTerminal = func(stream any) bool { return terminalOut && (stream == std.Out || stream == std.In) }
	return std, out, errOut
}

func TestStdinNotTerminalRefused(t *testing.T) {
	f := &fakeIssuer{}
	f.install(t, "ac04")
	out, errOut := &bytes.Buffer{}, &bytes.Buffer{}
	std := IO{In: strings.NewReader(""), Out: out, Err: errOut, Mode: ModeContainer}
	std.IsTerminal = func(stream any) bool { return stream == std.Out }
	if code := Run([]string{"jwt", "generate", "ac04"}, std); code != ExitFailure {
		t.Fatalf("exit = %d; want %d", code, ExitFailure)
	}
	if out.Len() != 0 {
		t.Fatalf("stdout = %q; want empty", out.String())
	}
	if !strings.Contains(errOut.String(), "terminal") {
		t.Fatalf("stderr = %q; want a terminal message", errOut.String())
	}
	if f.loaded || f.keyLoaded {
		t.Fatal("command loaded the config or the key without both terminals")
	}
}

func TestAC04_RefusesNoTerminal(t *testing.T) {
	f := &fakeIssuer{}
	f.install(t, "ac04")
	std, out, errOut := jwtIO(false)
	std.IsTerminal = func(stream any) bool { return stream != std.Out }
	if code := Run([]string{"jwt", "generate", "ac04"}, std); code != ExitFailure {
		t.Fatalf("exit = %d; want %d", code, ExitFailure)
	}
	if out.Len() != 0 {
		t.Fatalf("stdout = %q; want empty", out.String())
	}
	if !strings.Contains(errOut.String(), "terminal") {
		t.Fatalf("stderr = %q; want a terminal message", errOut.String())
	}
	if f.loaded || f.keyLoaded {
		t.Fatal("command loaded the config or the key without a terminal")
	}
}

func TestAC04_RefusesUnknownSub(t *testing.T) {
	f := &fakeIssuer{}
	f.install(t, "ac04")
	std, out, errOut := jwtIO(true)
	if code := Run([]string{"jwt", "generate", "nobody"}, std); code != ExitFailure {
		t.Fatalf("exit = %d; want %d", code, ExitFailure)
	}
	if out.Len() != 0 {
		t.Fatalf("stdout = %q; want empty", out.String())
	}
	if !strings.Contains(errOut.String(), "nobody") {
		t.Fatalf("stderr = %q; want the refused sub", errOut.String())
	}
	if f.keyLoaded {
		t.Fatal("command loaded the key for an unknown sub")
	}
}

func TestAC04_RefusesIssuerLoadError(t *testing.T) {
	saved := loadJWTIssuer
	t.Cleanup(func() { loadJWTIssuer = saved })
	loadJWTIssuer = func() (jwtIssuer, error) { return jwtIssuer{}, errors.New("no config") }
	std, out, _ := jwtIO(true)
	if code := Run([]string{"jwt", "generate", "ac04"}, std); code != ExitFailure {
		t.Fatalf("exit = %d; want %d", code, ExitFailure)
	}
	if out.Len() != 0 {
		t.Fatalf("stdout = %q; want empty", out.String())
	}
}

func TestAC04_RefusesBadUsage(t *testing.T) {
	f := &fakeIssuer{}
	f.install(t, "ac04")
	for _, args := range [][]string{
		{"jwt", "generate"},
		{"jwt", "generate", "ac04", "extra"},
		{"jwt", "generate", "ac04", "--name"},
		{"jwt", "generate", "ac04", "--unknown", "x"},
	} {
		std, out, _ := jwtIO(true)
		if code := Run(args, std); code != ExitUsage {
			t.Fatalf("Run(%q) = %d; want %d", args, code, ExitUsage)
		}
		if out.Len() != 0 {
			t.Fatalf("Run(%q) stdout = %q; want empty", args, out.String())
		}
	}
}

func TestPrintsBareJWT(t *testing.T) {
	cases := []struct {
		args     []string
		wantName string
	}{
		{[]string{"jwt", "generate", "ac04"}, "ac04"},
		{[]string{"jwt", "generate", "ac04", "--name", "AC 04"}, "AC 04"},
		{[]string{"jwt", "generate", "ac04", "--name=AC 04"}, "AC 04"},
	}
	for _, tc := range cases {
		f := &fakeIssuer{}
		f.install(t, "ac04")
		std, out, errOut := jwtIO(true)
		if code := Run(tc.args, std); code != ExitOK {
			t.Fatalf("Run(%q) = %d; stderr %q", tc.args, code, errOut.String())
		}
		text := out.String()
		if !strings.HasSuffix(text, "\n") || strings.Count(text, "\n") != 1 {
			t.Fatalf("stdout = %q; want one line", text)
		}
		raw := strings.TrimSuffix(text, "\n")
		if strings.ContainsAny(raw, " \t\r") || strings.Count(raw, ".") != 2 {
			t.Fatalf("stdout = %q; want a bare JWT", text)
		}
		if strings.Contains(errOut.String(), raw) {
			t.Fatal("stderr holds the token")
		}
		claims, err := token.Verify(jwtTestKey, raw, time.Now())
		if err != nil {
			t.Fatalf("Verify: %v", err)
		}
		if claims.Sub != "ac04" || claims.Name != tc.wantName {
			t.Fatalf("claims = %+v; want sub ac04, name %q", claims, tc.wantName)
		}
		if got := claims.ExpiresAt.Sub(claims.IssuedAt); got != time.Hour {
			t.Fatalf("lifetime = %v; want 1h", got)
		}
	}
}
