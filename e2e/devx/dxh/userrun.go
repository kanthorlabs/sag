package dxh

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"time"
)

type UserSnapshot struct {
	Dir    string
	Absent map[string]bool
	home   string
	shell  string
	zdot   string
}

func CheckNoClaude(ctx context.Context) error {
	out, err := exec.CommandContext(ctx, "ps", "aux").Output()
	if err != nil {
		return fmt.Errorf("checknoclaude: ps aux: %w", err)
	}
	return ScanForClaudeProcess(string(out))
}

func ScanForClaudeProcess(psOutput string) error {
	for _, line := range strings.Split(psOutput, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 11 {
			continue
		}
		if fields[1] == "PID" {
			continue
		}
		cmd := fields[10]
		base := filepath.Base(cmd)
		if base == "claude" {
			return fmt.Errorf("checknoclaude: claude process is running; stop it before running the User suite")
		}
		if base == "node" {
			for _, arg := range fields[11:] {
				if strings.Contains(arg, "claude") {
					return fmt.Errorf("checknoclaude: node process with claude in arguments is running; stop it before running the User suite")
				}
			}
		}
	}
	return nil
}

func CheckProfileEnv() error {
	if v := os.Getenv("CLAUDE_CONFIG_DIR"); v != "" {
		return fmt.Errorf("unset CLAUDE_CONFIG_DIR; this suite uses the default profile")
	}
	if v := os.Getenv("PI_CODING_AGENT_DIR"); v != "" {
		return fmt.Errorf("unset PI_CODING_AGENT_DIR; this suite uses the default profile")
	}
	return nil
}

func Snapshot(dir string) (UserSnapshot, error) {
	home := os.Getenv("HOME")
	shell := os.Getenv("SHELL")
	zdot := os.Getenv("ZDOTDIR")

	if err := os.MkdirAll(dir, 0o700); err != nil {
		return UserSnapshot{}, fmt.Errorf("snapshot mkdir: %w", err)
	}

	snap := UserSnapshot{
		Dir:    dir,
		Absent: map[string]bool{},
		home:   home,
		shell:  shell,
		zdot:   zdot,
	}

	sources := map[string]string{
		"claude.json":          filepath.Join(home, ".claude.json"),
		"claude-settings.json": filepath.Join(home, ".claude", "settings.json"),
		"pi-mcp.json":          filepath.Join(home, ".pi", "agent", "mcp.json"),
	}
	profile, hasProfile := userShellProfile(runtime.GOOS, shell, zdot, home)
	if hasProfile {
		sources["shell"] = profile
	}

	for name, src := range sources {
		if _, err := os.Stat(src); os.IsNotExist(err) {
			snap.Absent[name] = true
			continue
		}
		data, err := os.ReadFile(src)
		if err != nil {
			return UserSnapshot{}, fmt.Errorf("snapshot read %s: %w", src, err)
		}
		dst := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
			return UserSnapshot{}, fmt.Errorf("snapshot mkdir for %s: %w", name, err)
		}
		if err := os.WriteFile(dst, data, 0o600); err != nil {
			return UserSnapshot{}, fmt.Errorf("snapshot write %s: %w", name, err)
		}
	}
	return snap, nil
}

func CompareSAGEntries(s UserSnapshot) error {
	home := s.home
	if home == "" {
		home = os.Getenv("HOME")
	}
	shell := s.shell
	if shell == "" {
		shell = os.Getenv("SHELL")
	}
	zdot := s.zdot
	if zdot == "" {
		zdot = os.Getenv("ZDOTDIR")
	}

	type check struct {
		key     string
		snapSrc string
		liveSrc string
		extract func([]byte) (interface{}, error)
	}

	profile, hasProfile := userShellProfile(runtime.GOOS, shell, zdot, home)

	checks := []check{
		{
			key:     "claude.json",
			snapSrc: filepath.Join(s.Dir, "claude.json"),
			liveSrc: filepath.Join(home, ".claude.json"),
			extract: extractMCPServersSAG,
		},
		{
			key:     "claude-settings.json",
			snapSrc: filepath.Join(s.Dir, "claude-settings.json"),
			liveSrc: filepath.Join(home, ".claude", "settings.json"),
			extract: extractClaudeSettingsSAG,
		},
		{
			key:     "pi-mcp.json",
			snapSrc: filepath.Join(s.Dir, "pi-mcp.json"),
			liveSrc: filepath.Join(home, ".pi", "agent", "mcp.json"),
			extract: extractMCPServersSAG,
		},
	}
	if hasProfile {
		checks = append(checks, check{
			key:     "shell",
			snapSrc: filepath.Join(s.Dir, "shell"),
			liveSrc: profile,
			extract: extractShellSAGLines,
		})
	}

	var diffs []string
	for _, c := range checks {
		snapEntry, err := readSAGEntry(s.Absent[c.key], c.snapSrc, c.extract)
		if err != nil {
			diffs = append(diffs, fmt.Sprintf("%s: snapshot read: %s", c.key, err))
			continue
		}
		liveAbsent := false
		if _, err := os.Stat(c.liveSrc); os.IsNotExist(err) {
			liveAbsent = true
		}
		liveEntry, err := readSAGEntry(liveAbsent, c.liveSrc, c.extract)
		if err != nil {
			diffs = append(diffs, fmt.Sprintf("%s: live read: %s", c.key, err))
			continue
		}
		if !reflect.DeepEqual(snapEntry, liveEntry) {
			diffs = append(diffs, fmt.Sprintf("%s: SAG entries differ", c.key))
		}
	}

	if len(diffs) > 0 {
		return fmt.Errorf("comparesagentries: %s; copy directory: %s", strings.Join(diffs, "; "), s.Dir)
	}
	_ = os.RemoveAll(s.Dir)
	return nil
}

func readSAGEntry(absent bool, path string, extract func([]byte) (interface{}, error)) (interface{}, error) {
	if absent {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return extract(data)
}

func extractMCPServersSAG(data []byte) (interface{}, error) {
	var top map[string]json.RawMessage
	if err := json.Unmarshal(data, &top); err != nil {
		return nil, err
	}
	raw, ok := top["mcpServers"]
	if !ok {
		return nil, nil
	}
	var servers map[string]json.RawMessage
	if err := json.Unmarshal(raw, &servers); err != nil {
		return nil, err
	}
	sag, ok := servers["sag"]
	if !ok {
		return nil, nil
	}
	var v interface{}
	if err := json.Unmarshal(sag, &v); err != nil {
		return nil, err
	}
	return v, nil
}

type sagSettings struct {
	NodeCA    string
	DenyRules []string
}

func extractClaudeSettingsSAG(data []byte) (interface{}, error) {
	var top map[string]json.RawMessage
	if err := json.Unmarshal(data, &top); err != nil {
		return nil, err
	}

	result := sagSettings{}

	if envRaw, ok := top["env"]; ok {
		var env map[string]string
		if err := json.Unmarshal(envRaw, &env); err == nil {
			result.NodeCA = env["NODE_EXTRA_CA_CERTS"]
		}
	}

	if permRaw, ok := top["permissions"]; ok {
		var perms map[string]json.RawMessage
		if err := json.Unmarshal(permRaw, &perms); err == nil {
			if denyRaw, ok := perms["deny"]; ok {
				var deny []string
				if err := json.Unmarshal(denyRaw, &deny); err == nil {
					for _, rule := range deny {
						if isSAGDenyRule(rule) {
							result.DenyRules = append(result.DenyRules, rule)
						}
					}
					sort.Strings(result.DenyRules)
				}
			}
		}
	}

	if result.NodeCA == "" && len(result.DenyRules) == 0 {
		return nil, nil
	}
	return result, nil
}

func isSAGDenyRule(rule string) bool {
	return strings.HasPrefix(rule, "Bash(sag credential delete") ||
		strings.HasPrefix(rule, "Bash(podman secret rm") ||
		strings.HasPrefix(rule, "Bash(script")
}

func extractShellSAGLines(data []byte) (interface{}, error) {
	var lines []string
	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) == "# sag agent setup" ||
			strings.HasPrefix(strings.TrimSpace(line), "export NODE_EXTRA_CA_CERTS=") {
			lines = append(lines, line)
		}
	}
	if len(lines) == 0 {
		return nil, nil
	}
	sort.Strings(lines)
	return lines, nil
}

func TeardownHarnessFiles(ctx context.Context, r Runner, home string) error {
	shell := os.Getenv("SHELL")
	zdot := os.Getenv("ZDOTDIR")
	var errs []string

	if _, err := r.Output(ctx, "claude", "mcp", "remove", "sag", "-s", "user"); err != nil {
		errs = append(errs, fmt.Sprintf("claude mcp remove sag: %s", err))
	}
	if err := removeClaudeSettingsSAGEntries(filepath.Join(home, ".claude", "settings.json")); err != nil {
		errs = append(errs, fmt.Sprintf("claude settings sag entries: %s", err))
	}
	if err := os.RemoveAll(filepath.Join(home, ".claude", "skills", "sag")); err != nil {
		errs = append(errs, fmt.Sprintf("claude skills sag: %s", err))
	}
	if err := removeClaudeBackupsWithSAG(filepath.Join(home, ".claude", "backups")); err != nil {
		errs = append(errs, fmt.Sprintf("claude backups: %s", err))
	}

	if _, err := r.Output(ctx, "pi", "mcp", "remove", "sag"); err != nil {
		errs = append(errs, fmt.Sprintf("pi mcp remove sag: %s", err))
	}
	if err := os.RemoveAll(filepath.Join(home, ".pi", "skills", "sag")); err != nil {
		errs = append(errs, fmt.Sprintf("pi skills sag: %s", err))
	}
	if err := os.Remove(filepath.Join(home, ".pi", "extensions", "sag-guard.ts")); err != nil && !os.IsNotExist(err) {
		errs = append(errs, fmt.Sprintf("pi extensions sag-guard.ts: %s", err))
	}

	profile, hasProfile := userShellProfile(runtime.GOOS, shell, zdot, home)
	if hasProfile {
		if err := removeShellSAGLines(profile); err != nil {
			errs = append(errs, fmt.Sprintf("shell profile sag lines: %s", err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("teardown harness files: %s", strings.Join(errs, "; "))
	}
	return nil
}

func removeClaudeSettingsSAGEntries(path string) error {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(data, &top); err != nil {
		return err
	}
	changed := false

	if envRaw, ok := top["env"]; ok {
		var env map[string]json.RawMessage
		if err := json.Unmarshal(envRaw, &env); err == nil {
			if _, has := env["NODE_EXTRA_CA_CERTS"]; has {
				delete(env, "NODE_EXTRA_CA_CERTS")
				changed = true
				if len(env) == 0 {
					delete(top, "env")
				} else {
					b, _ := json.Marshal(env)
					top["env"] = b
				}
			}
		}
	}

	if permRaw, ok := top["permissions"]; ok {
		var perms map[string]json.RawMessage
		if err := json.Unmarshal(permRaw, &perms); err == nil {
			if denyRaw, ok := perms["deny"]; ok {
				var deny []string
				if err := json.Unmarshal(denyRaw, &deny); err == nil {
					var kept []string
					for _, rule := range deny {
						if !isSAGDenyRule(rule) {
							kept = append(kept, rule)
						}
					}
					if len(kept) != len(deny) {
						changed = true
						if len(kept) == 0 {
							delete(perms, "deny")
						} else {
							b, _ := json.Marshal(kept)
							perms["deny"] = b
						}
						if len(perms) == 0 {
							delete(top, "permissions")
						} else {
							b, _ := json.Marshal(perms)
							top["permissions"] = b
						}
					}
				}
			}
		}
	}

	if !changed {
		return nil
	}
	return writeJSONFile(path, top)
}

func removeClaudeBackupsWithSAG(backupsDir string) error {
	entries, err := os.ReadDir(backupsDir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		p := filepath.Join(backupsDir, e.Name())
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		var top map[string]json.RawMessage
		if err := json.Unmarshal(data, &top); err != nil {
			continue
		}
		if _, ok := top["mcpServers"]; !ok {
			continue
		}
		var servers map[string]json.RawMessage
		if err := json.Unmarshal(top["mcpServers"], &servers); err != nil {
			continue
		}
		if _, hasSAG := servers["sag"]; hasSAG {
			_ = os.Remove(p)
		}
	}
	return nil
}

func removeShellSAGLines(path string) error {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	lines := strings.Split(string(data), "\n")
	var kept []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "# sag agent setup" || strings.HasPrefix(trimmed, "export NODE_EXTRA_CA_CERTS=") {
			continue
		}
		kept = append(kept, line)
	}
	if len(kept) == len(lines) {
		return nil
	}
	out := strings.Join(kept, "\n")
	return writeTextFile(path, []byte(out))
}

func writeJSONFile(path string, v interface{}) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".sag-tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	info, err := os.Stat(path)
	if err == nil {
		_ = os.Chmod(tmpName, info.Mode())
	}
	return os.Rename(tmpName, path)
}

func writeTextFile(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".sag-tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	info, err := os.Stat(path)
	if err == nil {
		_ = os.Chmod(tmpName, info.Mode())
	}
	return os.Rename(tmpName, path)
}

func MaskTailscaleIP(line, ip string) string {
	if ip == "" {
		return line
	}
	return strings.ReplaceAll(line, ip, "<tailscale IP>")
}

func userShellProfile(goos, shell, zdotdir, home string) (string, bool) {
	base := filepath.Base(shell)
	switch base {
	case "zsh":
		dir := home
		if zdotdir != "" {
			dir = zdotdir
		}
		return filepath.Join(dir, ".zshrc"), true
	case "bash":
		if goos == "darwin" {
			return filepath.Join(home, ".bash_profile"), true
		}
		return filepath.Join(home, ".bashrc"), true
	default:
		return "", false
	}
}

func UserSnapshotDir() (string, error) {
	devDir, err := SharedDevDir()
	if err != nil {
		return "", err
	}
	ts := time.Now().UTC().Format("20060102T150405Z")
	return filepath.Join(devDir, "local", "devx-user", ts), nil
}
