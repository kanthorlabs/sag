package dxh_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kanthorlabs/sag/e2e/devx/dxh"
)

func TestSharedDevDirIsMainRepository(t *testing.T) {
	dir, err := dxh.SharedDevDir()
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(dir) != ".dev" {
		t.Fatalf("SharedDevDir = %q, want a .dev directory", dir)
	}
	info, err := os.Stat(filepath.Join(filepath.Dir(dir), ".git"))
	if err != nil || !info.IsDir() {
		t.Fatalf("parent of %q has no .git directory; a worktree has a .git file", dir)
	}
}
