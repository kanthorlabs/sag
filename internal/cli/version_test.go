package cli

import (
	"bytes"
	"testing"
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
	if got := out.String(); got != "sag v1.2.3-test\n" {
		t.Fatalf("stdout = %q; want one version line", got)
	}
}

func TestVersionRejectsArgs(t *testing.T) {
	std, _ := testIO()
	if code := Run([]string{"version", "extra"}, std); code != ExitUsage {
		t.Fatalf("Run(version extra) = %d; want %d", code, ExitUsage)
	}
}
