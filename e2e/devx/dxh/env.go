package dxh

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var allowedEnvKeys = map[string]bool{
	"HOME":   true,
	"TMPDIR": true,
	"USER":   true,
	"LANG":   true,
}

func ChildEnv(env []string, groupDir string) ([]string, error) {
	if groupDir == "" {
		return nil, fmt.Errorf("groupDir is empty")
	}
	var out []string
	for _, e := range env {
		key, _, _ := strings.Cut(e, "=")
		if allowedEnvKeys[key] || strings.HasPrefix(key, "SAG_TEST_") {
			out = append(out, e)
		}
	}
	claudeDir := filepath.Join(groupDir, "claude")
	piDir := filepath.Join(groupDir, "pi")
	if err := os.MkdirAll(claudeDir, 0o700); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(piDir, 0o700); err != nil {
		return nil, err
	}
	out = append(out, "CLAUDE_CONFIG_DIR="+claudeDir)
	out = append(out, "PI_CODING_AGENT_DIR="+piDir)
	return out, nil
}
