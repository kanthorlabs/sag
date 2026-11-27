package cli

import (
	"bytes"
	"testing"

	"github.com/kanthorlabs/sag/internal/deploy"
)

func runVersion(t *testing.T, mode Mode) string {
	t.Helper()
	std, _ := testIO()
	std.Mode = mode
	out := &bytes.Buffer{}
	std.Out = out
	if code := Run([]string{"version"}, std); code != ExitOK {
		t.Fatalf("Run(version) = %d; want %d", code, ExitOK)
	}
	return out.String()
}

func TestPrintsImageReference(t *testing.T) {
	t.Setenv("SAG_TEST_IMAGE", "")
	t.Setenv("SAG_TEST_REGISTRY", "")
	savedVersion, savedDigest := Version, deploy.ImageDigest
	defer func() { Version, deploy.ImageDigest = savedVersion, savedDigest }()
	Version = "v0.0.0-imageref"

	deploy.ImageDigest = ""
	if got, want := runVersion(t, ModeHost), "sag v0.0.0-imageref docker.io/kanthorlabs/sag:main\n"; got != want {
		t.Fatalf("dev build stdout = %q; want %q", got, want)
	}
	deploy.ImageDigest = "sha256:0123abcd"
	if got, want := runVersion(t, ModeHost), "sag v0.0.0-imageref docker.io/kanthorlabs/sag@sha256:0123abcd\n"; got != want {
		t.Fatalf("release stdout = %q; want %q", got, want)
	}
}

func TestVersionInContainerPrintsOnlyVersion(t *testing.T) {
	saved := Version
	defer func() { Version = saved }()
	Version = "v1.2.3-test"
	if got, want := runVersion(t, ModeContainer), "sag v1.2.3-test\n"; got != want {
		t.Fatalf("stdout = %q; want %q", got, want)
	}
}

func TestVersionRejectsArgs(t *testing.T) {
	std, _ := testIO()
	if code := Run([]string{"version", "extra"}, std); code != ExitUsage {
		t.Fatalf("Run(version extra) = %d; want %d", code, ExitUsage)
	}
}
