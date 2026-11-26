//go:build devxe2e

package devx_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func worktreeRoot() (string, error) {
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func testDXAC33_ProductBinary(t *testing.T, ctx context.Context, groupDir, binDir string) {
	t.Run("DXAC33_ProductBinary", func(t *testing.T) {
		root, err := worktreeRoot()
		if err != nil {
			t.Fatalf("worktreeRoot: %v", err)
		}
		sagBin := filepath.Join(root, "bin", "sag")

		if _, err := os.Stat(sagBin); err != nil {
			t.Fatalf("product binary not found: %v", err)
		}

		nmOut, err := exec.CommandContext(ctx, "go", "tool", "nm", sagBin).Output()
		if err != nil {
			t.Fatalf("go tool nm: %v", err)
		}
		if strings.Contains(string(nmOut), "devxtest") {
			t.Fatal("product binary symbol table contains devxtest")
		}

		err = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				base := d.Name()
				if base == ".git" || base == "e2e" || strings.HasPrefix(base, ".") {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if strings.Contains(string(raw), "--showsecret") {
				t.Errorf("non-test source %s contains --showsecret", path)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("WalkDir: %v", err)
		}
	})
}
