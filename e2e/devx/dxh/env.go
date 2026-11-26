package dxh

import (
	"os"
	"path/filepath"
	"strings"
)

var allowedEnvKeys = map[string]bool{
	"HOME":    true,
	"TMPDIR":  true,
	"USER":    true,
	"LANG":    true,
}

func ChildEnv(env []string, groupDir string) []string {
	var out []string
	for _, e := range env {
		key, _, _ := strings.Cut(e, "=")
		if allowedEnvKeys[key] || strings.HasPrefix(key, "SAG_TEST_") {
			out = append(out, e)
		}
	}
	if groupDir != "" {
		claudeDir := filepath.Join(groupDir, "claude")
		piDir := filepath.Join(groupDir, "pi")
		_ = os.MkdirAll(claudeDir, 0o700)
		_ = os.MkdirAll(piDir, 0o700)
		out = append(out, "CLAUDE_CONFIG_DIR="+claudeDir)
		out = append(out, "PI_CODING_AGENT_DIR="+piDir)
	}
	return out
}
