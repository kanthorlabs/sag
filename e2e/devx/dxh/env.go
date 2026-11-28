package dxh

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
)

var allowedEnvKeys = map[string]bool{
	"HOME":   true,
	"TMPDIR": true,
	"USER":   true,
	"LANG":   true,
}

var awsTestEnvKeys = map[string]bool{
	"AWS_ENDPOINT_URL_SECRETS_MANAGER": true,
	"AWS_ACCESS_KEY_ID":                true,
	"AWS_SECRET_ACCESS_KEY":            true,
	"AWS_REGION":                       true,
	"AWS_CONFIG_FILE":                  true,
	"AWS_SHARED_CREDENTIALS_FILE":      true,
}

func ChildEnv(env []string, groupDir string) ([]string, error) {
	if groupDir == "" {
		return nil, fmt.Errorf("groupDir is empty")
	}
	awsTest := slices.ContainsFunc(env, func(e string) bool { return strings.HasPrefix(e, "SAG_TEST_AWS_ENDPOINT=") })
	var out []string
	for _, e := range env {
		key, _, _ := strings.Cut(e, "=")
		if allowedEnvKeys[key] || (runtime.GOOS == "linux" && key == "XDG_RUNTIME_DIR") || strings.HasPrefix(key, "SAG_TEST_") || (awsTest && awsTestEnvKeys[key]) {
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
