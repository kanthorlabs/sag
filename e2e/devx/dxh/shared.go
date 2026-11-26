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
