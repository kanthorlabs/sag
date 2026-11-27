package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/kanthorlabs/sag/internal/image"
)

func TestVersionPrintsOneLine(t *testing.T) {
	saved := Version
	defer func() { Version = saved }()
	Version = "v1.2.3-test"
	std, _ := testIO()
	out := &bytes.Buffer{}
	std.Out = out
	if code := Run([]string{"version"}, std); code != ExitOK {
		t.Fatalf("Run(version) = %d; want %d", code, ExitOK)
	}
	got := out.String()
	if !strings.HasPrefix(got, "sag v1.2.3-test ") {
		t.Fatalf("stdout = %q; want prefix 'sag v1.2.3-test '", got)
	}
}

func TestPrintsImageReference(t *testing.T) {
	t.Setenv("SAG_TEST_IMAGE", "")
	t.Setenv("SAG_TEST_REGISTRY", "")
	saved := Version
	defer func() { Version = saved }()
	Version = "v0.0.0-imageref"
	std, _ := testIO()
	out := &bytes.Buffer{}
	std.Out = out
	if code := Run([]string{"version"}, std); code != ExitOK {
		t.Fatalf("Run(version) = %d; want %d", code, ExitOK)
	}
	ref, _ := image.Reference()
	want := "sag v0.0.0-imageref " + ref + "\n"
	if got := out.String(); got != want {
		t.Fatalf("stdout = %q; want %q", got, want)
	}
}

func TestVersionRejectsArgs(t *testing.T) {
	std, _ := testIO()
	if code := Run([]string{"version", "extra"}, std); code != ExitUsage {
		t.Fatalf("Run(version extra) = %d; want %d", code, ExitUsage)
	}
}
