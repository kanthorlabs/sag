package dxh

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

func SharedDevDir() (string, error) {
	out, err := exec.Command("git", "rev-parse", "--path-format=absolute", "--git-common-dir").Output()
	if err != nil {
		return "", fmt.Errorf("git rev-parse --git-common-dir: %w", err)
	}
	common := strings.TrimSpace(string(out))
	if !filepath.IsAbs(common) || filepath.Base(common) != ".git" {
		return "", fmt.Errorf("unexpected git common dir %q", common)
	}
	return filepath.Join(filepath.Dir(common), ".dev"), nil
}

func TestBinary() (string, error) {
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", fmt.Errorf("git rev-parse --show-toplevel: %w", err)
	}
	return filepath.Join(strings.TrimSpace(string(out)), "bin", "sag-devxtest"), nil
}

func RepoRoot() (string, error) {
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", fmt.Errorf("git rev-parse --show-toplevel: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}
