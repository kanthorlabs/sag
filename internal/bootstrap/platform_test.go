package bootstrap

import (
	"context"
	"io"
	"testing"

	"github.com/kanthorlabs/sag/internal/stepline"
)

func TestRuledLinuxAccepted(t *testing.T) {
	if err := checkLinuxDistro("linux", "amd64", "debian", "13"); err != nil {
		t.Fatalf("checkLinuxDistro debian 13 amd64: %v", err)
	}
	if err := checkLinuxDistro("linux", "arm64", "debian", "13"); err != nil {
		t.Fatalf("checkLinuxDistro debian 13 arm64: %v", err)
	}
}

func TestOtherDistributionExit1(t *testing.T) {
	cases := []struct {
		goarch    string
		id        string
		versionID string
	}{
		{"amd64", "ubuntu", "22.04"},
		{"amd64", "debian", "12"},
		{"amd64", "fedora", "39"},
		{"arm64", "ubuntu", "24.04"},
		{"amd64", "debian", "14"},
	}
	for _, tc := range cases {
		want := "unsupported platform linux/" + tc.goarch
		err := checkLinuxDistro("linux", tc.goarch, tc.id, tc.versionID)
		if err == nil {
			t.Fatalf("checkLinuxDistro linux/%s id=%s versionID=%s: want error, got nil", tc.goarch, tc.id, tc.versionID)
		}
		if err.Error() != want {
			t.Fatalf("checkLinuxDistro linux/%s id=%s versionID=%s: error = %q; want %q", tc.goarch, tc.id, tc.versionID, err.Error(), want)
		}
	}
}

func TestOtherDistributionStepExit1(t *testing.T) {
	deps := Deps{
		GOOS:   func() string { return "linux" },
		GOARCH: func() string { return "amd64" },
		OpenFile: func(path string) error {
			return io.ErrUnexpectedEOF
		},
		UID:        func() string { return "1000" },
		Home:       func() string { return "/home/user" },
		SAGPath:    func() (string, error) { return "/usr/local/bin/sag", nil },
		OSRelease:  func() (string, string, error) { return "ubuntu", "22.04", nil },
		Out:        stepline.New(io.Discard),
		Print:      io.Discard,
	}
	steps := deps.BuildSteps()
	s := &State{Out: deps.Out}
	err := steps[0].Run(context.Background(), s)
	if err == nil {
		t.Fatal("step platform-check: want error for ubuntu, got nil")
	}
	if err.Error() != "unsupported platform linux/amd64" {
		t.Fatalf("step platform-check error = %q; want %q", err.Error(), "unsupported platform linux/amd64")
	}
}

func TestParseOSRelease(t *testing.T) {
	content := `ID=debian
VERSION_ID="13"
PRETTY_NAME="Debian GNU/Linux 13 (trixie)"
`
	id, versionID, err := parseOSRelease(content)
	if err != nil {
		t.Fatalf("parseOSRelease: %v", err)
	}
	if id != "debian" {
		t.Fatalf("id = %q; want %q", id, "debian")
	}
	if versionID != "13" {
		t.Fatalf("versionID = %q; want %q", versionID, "13")
	}
}
