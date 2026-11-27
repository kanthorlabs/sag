package agentsetup

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const (
	denyRuleSagDelete = "Bash(sag credential delete:*)"
	denyRulePodmanRM  = "Bash(podman secret rm:*)"
	denyRuleScript    = "Bash(script:*)"
	guardMarker       = "// managed by sag agent setup"
)

var sagDenyRules = []string{denyRuleSagDelete, denyRulePodmanRM, denyRuleScript}

var writeJSONFn func(path string, keyPath []string, value any) error = writeJSON

func applyDenyRules(ctx context.Context, opts Options, dir string) error {
	if opts.DenyRules != "yes" {
		return nil
	}
	settingsPath := filepath.Join(dir, "settings.json")
	current, err := readDenyRules(settingsPath)
	if err != nil {
		return err
	}
	next := appendMissingRules(current, sagDenyRules)
	if len(next) == len(current) {
		return nil
	}
	return writeJSONFn(settingsPath, []string{"permissions", "deny"}, next)
}

func readDenyRules(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var top struct {
		Permissions struct {
			Deny []string `json:"deny"`
		} `json:"permissions"`
	}
	if err := json.Unmarshal(data, &top); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return top.Permissions.Deny, nil
}

func appendMissingRules(current, toAdd []string) []string {
	out := append([]string(nil), current...)
	for _, rule := range toAdd {
		if !containsString(out, rule) {
			out = append(out, rule)
		}
	}
	return out
}

func containsString(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}

func applyGuard(ctx context.Context, opts Options, dir string, content []byte) error {
	if opts.Guard != "yes" {
		return nil
	}
	extDir := filepath.Join(dir, "extensions")
	if err := os.MkdirAll(extDir, 0o700); err != nil {
		return fmt.Errorf("create extensions dir: %w", err)
	}
	guardPath := filepath.Join(extDir, "sag-guard.ts")
	mode := fs.FileMode(0o600)
	if info, err := os.Stat(guardPath); err == nil {
		line1, lineErr := readFirstLine(guardPath)
		if lineErr != nil {
			return lineErr
		}
		if line1 != guardMarker {
			return fmt.Errorf("fail sag-guard.ts in %s has another form; remove it", guardPath)
		}
		mode = info.Mode()
	}
	return os.WriteFile(guardPath, content, mode)
}

func readFirstLine(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	if sc.Scan() {
		return strings.TrimRight(sc.Text(), "\r"), nil
	}
	return "", sc.Err()
}
