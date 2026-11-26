package pins_test

import (
	"errors"
	"testing"

	"github.com/kanthorlabs/sag/internal/pins"
)

func TestParseLine(t *testing.T) {
	tests := []struct {
		name    string
		line    string
		want    pins.Item
		wantErr bool
	}{
		{
			name: "valid release line",
			line: "podman 5.5.1 darwin/arm64 release https://example.com/podman.pkg abc123",
			want: pins.Item{
				Name: "podman", Version: "5.5.1", Platform: "darwin/arm64",
				Source: "release", Locator: "https://example.com/podman.pkg", SHA256: "abc123",
			},
		},
		{
			name: "valid apt line with dash",
			line: "podman 4.9.3 linux/* apt podman -",
			want: pins.Item{
				Name: "podman", Version: "4.9.3", Platform: "linux/*",
				Source: "apt", Locator: "podman", SHA256: "-",
			},
		},
		{
			name:    "empty line",
			line:    "",
			wantErr: true,
		},
		{
			name:    "comment line",
			line:    "# this is a comment",
			wantErr: true,
		},
		{
			name:    "too few fields",
			line:    "podman 5.5.1 darwin/arm64",
			wantErr: true,
		},
		{
			name:    "too many fields",
			line:    "podman 5.5.1 darwin/arm64 release https://x.com/a.pkg sha256 extra",
			wantErr: true,
		},
		{
			name:    "whitespace only",
			line:    "   ",
			wantErr: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := pins.ParseLine(tc.line)
			if (err != nil) != tc.wantErr {
				t.Fatalf("ParseLine(%q) error = %v, wantErr %v", tc.line, err, tc.wantErr)
			}
			if !tc.wantErr && got != tc.want {
				t.Fatalf("ParseLine(%q) = %+v, want %+v", tc.line, got, tc.want)
			}
		})
	}
}

func TestSelectPlatform(t *testing.T) {
	items := []pins.Item{
		{Name: "podman", Platform: "darwin/arm64"},
		{Name: "podman", Platform: "darwin/amd64"},
		{Name: "podman", Platform: "linux/*"},
	}

	got := pins.SelectPlatform(items, "darwin", "arm64")
	if len(got) != 1 || got[0].Platform != "darwin/arm64" {
		t.Fatalf("SelectPlatform darwin/arm64 = %v; want one darwin/arm64 item", got)
	}
}

func TestSelectPlatformNoMatch(t *testing.T) {
	items := []pins.Item{
		{Name: "podman", Platform: "darwin/arm64"},
	}

	got := pins.SelectPlatform(items, "linux", "amd64")
	if len(got) != 0 {
		t.Fatalf("SelectPlatform linux/amd64 = %v; want empty", got)
	}
}

func TestParseLineSkipError(t *testing.T) {
	for _, line := range []string{"", "   ", "# comment"} {
		_, err := pins.ParseLine(line)
		var skipErr *pins.SkipError
		if !errors.As(err, &skipErr) {
			t.Errorf("ParseLine(%q) err = %v; want SkipError", line, err)
		}
	}
}

func TestParseLineMalformedIsError(t *testing.T) {
	_, err := pins.ParseLine("only three fields here")
	if err == nil {
		t.Fatal("ParseLine(malformed): want error, got nil")
	}
	var skipErr *pins.SkipError
	if errors.As(err, &skipErr) {
		t.Fatal("ParseLine(malformed): got SkipError; want real error")
	}
}

func TestValidateItemOK(t *testing.T) {
	item := pins.Item{
		Name: "podman", Version: "5.5.1", Platform: "darwin/arm64",
		Source:  "release",
		Locator: "https://example.com/podman.pkg",
		SHA256:  "deadbeef00000000000000000000000000000000000000000000000000000000",
	}
	if err := pins.ValidateItem(item, true); err != nil {
		t.Fatalf("ValidateItem: %v", err)
	}
}

func TestValidateItemAptDash(t *testing.T) {
	item := pins.Item{
		Name: "podman", Version: "4.9.3", Platform: "linux/*",
		Source: "apt", Locator: "podman", SHA256: "-",
	}
	if err := pins.ValidateItem(item, true); err != nil {
		t.Fatalf("ValidateItem apt with dash: %v", err)
	}
}

func TestValidateItemUnknownSource(t *testing.T) {
	item := pins.Item{
		Name: "pkg", Source: "npm", SHA256: "-", Locator: "pkg",
	}
	if err := pins.ValidateItem(item, false); err == nil {
		t.Fatal("ValidateItem unknown source: want error, got nil")
	}
}

func TestValidateItemShortSHA(t *testing.T) {
	item := pins.Item{
		Name: "pkg", Source: "release", SHA256: "abc123", Locator: "https://example.com/x",
	}
	if err := pins.ValidateItem(item, false); err == nil {
		t.Fatal("ValidateItem short sha: want error, got nil")
	}
}

func TestValidateItemHTTPSRequired(t *testing.T) {
	item := pins.Item{
		Name:    "pkg",
		Source:  "release",
		SHA256:  "deadbeef00000000000000000000000000000000000000000000000000000000",
		Locator: "http://example.com/x.pkg",
	}
	if err := pins.ValidateItem(item, true); err == nil {
		t.Fatal("ValidateItem http locator with requireHTTPS: want error, got nil")
	}
	if err := pins.ValidateItem(item, false); err != nil {
		t.Fatalf("ValidateItem http locator without requireHTTPS: %v", err)
	}
}

func TestLinuxWildcard(t *testing.T) {
	items := []pins.Item{
		{Name: "podman", Platform: "darwin/arm64"},
		{Name: "podman", Platform: "linux/*"},
	}

	got := pins.SelectPlatform(items, "linux", "amd64")
	if len(got) != 1 || got[0].Platform != "linux/*" {
		t.Fatalf("SelectPlatform linux/amd64 = %v; want one linux/* item", got)
	}
}
