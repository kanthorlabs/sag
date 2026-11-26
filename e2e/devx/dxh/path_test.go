package dxh_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kanthorlabs/sag/e2e/devx/dxh"
)

func TestPathHasNoHarnessOrTailscale(t *testing.T) {
	binDir := t.TempDir()
	testPath := dxh.TestPATH(binDir)

	entries := strings.Split(testPath, string(filepath.ListSeparator))
	if len(entries) == 0 {
		t.Fatal("TestPATH returned empty PATH")
	}
	if entries[0] != binDir {
		t.Errorf("first PATH entry = %q, want %q", entries[0], binDir)
	}
	for _, entry := range entries {
		if strings.Contains(entry, "claude") || strings.Contains(entry, "/pi") || strings.Contains(entry, "tailscale") {
			t.Errorf("PATH entry %q contains a harness or tailscale path", entry)
		}
	}
	for _, entry := range entries {
		lower := strings.ToLower(entry)
		if strings.Contains(lower, "home") || strings.Contains(lower, "users") {
			allowed := entry == binDir
			if !allowed {
				t.Errorf("PATH entry %q looks like a user-specific path", entry)
			}
		}
	}
}

func TestRealProgramLinkDir(t *testing.T) {
	target := "/usr/bin/env"
	linkDir := t.TempDir()
	if err := dxh.AddRealProgramLink(linkDir, "env", target); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(linkDir, "env")
	info, err := os.Lstat(link)
	if err != nil {
		t.Fatalf("link not created: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Errorf("%q is not a symlink", link)
	}
	dest, err := os.Readlink(link)
	if err != nil {
		t.Fatal(err)
	}
	if dest != target {
		t.Errorf("link target = %q, want %q", dest, target)
	}
}
