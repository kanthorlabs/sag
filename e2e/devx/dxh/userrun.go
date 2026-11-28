package dxh

import (
	"bytes"
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

	"github.com/kanthorlabs/sag/internal/agentsetup"
)

type UserSnapshot struct {
	Dir    string
	Absent map[string]bool
	home   string
	shell  string
	zdot   string
	cfgDir string
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
	cfgDir := CfgDir(home)

	if err := os.MkdirAll(dir, 0o700); err != nil {
		return UserSnapshot{}, fmt.Errorf("snapshot mkdir: %w", err)
	}

	snap := UserSnapshot{
		Dir:    dir,
		Absent: map[string]bool{},
		home:   home,
		shell:  shell,
		zdot:   zdot,
		cfgDir: cfgDir,
	}

	cc := agentsetup.ClaudeCodePaths("", home)
	pi := agentsetup.PiPaths("", home)
	sources := map[string]string{
		"claude.json":          cc.MCPFile,
		"claude-settings.json": cc.SettingsFile,
		"pi-mcp.json":          pi.MCPFile,
	}
	profile, hasProfile := agentsetup.ShellProfile(runtime.GOOS, shell, zdot, home)
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

func CheckNoSAGEntries(home string) error {
	cfgDir := CfgDir(home)
	shell := os.Getenv("SHELL")
	zdot := os.Getenv("ZDOTDIR")
	cc := agentsetup.ClaudeCodePaths("", home)
	pi := agentsetup.PiPaths("", home)
	var found []string

	if data, err := os.ReadFile(cc.MCPFile); err == nil {
		if v, _ := extractMCPServersSAG(data); v != nil {
			found = append(found, cc.MCPFile+": mcpServers.sag")
		}
	}
	if data, err := os.ReadFile(cc.SettingsFile); err == nil {
		if v, _ := extractClaudeSettingsSAGWith(cfgDir)(data); v != nil {
			found = append(found, cc.SettingsFile+": SAG settings")
		}
	}
	if data, err := os.ReadFile(pi.MCPFile); err == nil {
		if v, _ := extractMCPServersSAG(data); v != nil {
			found = append(found, pi.MCPFile+": mcpServers.sag")
		}
	}
	if profile, hasProfile := agentsetup.ShellProfile(runtime.GOOS, shell, zdot, home); hasProfile {
		if data, err := os.ReadFile(profile); err == nil {
			if v, _ := extractShellSAGLinesWith(cfgDir)(data); v != nil {
				found = append(found, profile+": SAG profile lines")
			}
		}
	}
	skillDir := filepath.Dir(cc.SkillFile)
	if _, err := os.Stat(skillDir); err == nil {
		found = append(found, skillDir+": claude skills/sag exists")
	}
	piSkillDir := filepath.Dir(pi.SkillFile)
	if _, err := os.Stat(piSkillDir); err == nil {
		found = append(found, piSkillDir+": pi skills/sag exists")
	}
	if _, err := os.Stat(pi.GuardFile); err == nil {
		found = append(found, pi.GuardFile+": sag-guard.ts exists")
	}
	if len(found) > 0 {
		return fmt.Errorf("SAG entries found before run; remove them first: %s", strings.Join(found, "; "))
	}
	return nil
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
	cfgDir := s.cfgDir
	if cfgDir == "" {
		cfgDir = CfgDir(home)
	}

	cc := agentsetup.ClaudeCodePaths("", home)
	pi := agentsetup.PiPaths("", home)

	type check struct {
		key     string
		snapSrc string
		liveSrc string
		extract func([]byte) (interface{}, error)
	}

	checks := []check{
		{
			key:     "claude.json",
			snapSrc: filepath.Join(s.Dir, "claude.json"),
			liveSrc: cc.MCPFile,
			extract: extractMCPServersSAG,
		},
		{
			key:     "claude-settings.json",
			snapSrc: filepath.Join(s.Dir, "claude-settings.json"),
			liveSrc: cc.SettingsFile,
			extract: extractClaudeSettingsSAGWith(cfgDir),
		},
		{
			key:     "pi-mcp.json",
			snapSrc: filepath.Join(s.Dir, "pi-mcp.json"),
			liveSrc: pi.MCPFile,
			extract: extractMCPServersSAG,
		},
	}
	if profile, hasProfile := agentsetup.ShellProfile(runtime.GOOS, shell, zdot, home); hasProfile {
		checks = append(checks, check{
			key:     "shell",
			snapSrc: filepath.Join(s.Dir, "shell"),
			liveSrc: profile,
			extract: extractShellSAGLinesWith(cfgDir),
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

func extractClaudeSettingsSAGWith(cfgDir string) func([]byte) (interface{}, error) {
	caPath := cfgDir + "/ca.pem"
	return func(data []byte) (interface{}, error) {
		var top map[string]json.RawMessage
		if err := json.Unmarshal(data, &top); err != nil {
			return nil, err
		}

		result := sagSettings{}

		if envRaw, ok := top["env"]; ok {
			var env map[string]string
			if err := json.Unmarshal(envRaw, &env); err == nil {
				if v := env["NODE_EXTRA_CA_CERTS"]; v == caPath {
					result.NodeCA = v
				}
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
}

const (
	sagDenyCredentialDelete = "Bash(sag credential delete:*)"
	sagDenyPodmanSecretRm   = "Bash(podman secret rm:*)"
	sagDenyScript           = "Bash(script:*)"
)

func isSAGDenyRule(rule string) bool {
	return rule == sagDenyCredentialDelete || rule == sagDenyPodmanSecretRm || rule == sagDenyScript
}

func extractShellSAGLinesWith(cfgDir string) func([]byte) (interface{}, error) {
	exactLines := map[string]bool{
		"# sag agent setup": true,
		`export NODE_EXTRA_CA_CERTS="` + cfgDir + `/ca.pem"`: true,
	}
	return func(data []byte) (interface{}, error) {
		var lines []string
		for _, line := range strings.Split(string(data), "\n") {
			if exactLines[strings.TrimSpace(line)] {
				lines = append(lines, line)
			}
		}
		if len(lines) == 0 {
			return nil, nil
		}
		sort.Strings(lines)
		return lines, nil
	}
}

func TeardownHarnessFiles(ctx context.Context, r Runner, home string) error {
	cfgDir := CfgDir(home)
	shell := os.Getenv("SHELL")
	zdot := os.Getenv("ZDOTDIR")
	cc := agentsetup.ClaudeCodePaths("", home)
	pi := agentsetup.PiPaths("", home)
	var errs []string

	if _, err := r.Output(ctx, "claude", "mcp", "remove", "sag", "-s", "user"); err != nil {
		errs = append(errs, fmt.Sprintf("claude mcp remove sag: %s", err))
	}
	if err := removeClaudeSettingsSAGEntries(cc.SettingsFile, cfgDir); err != nil {
		errs = append(errs, fmt.Sprintf("claude settings sag entries: %s", err))
	}
	if err := os.RemoveAll(filepath.Dir(cc.SkillFile)); err != nil {
		errs = append(errs, fmt.Sprintf("claude skills sag: %s", err))
	}
	if err := removeClaudeBackupsWithSAG(filepath.Join(cc.Dir, "backups")); err != nil {
		errs = append(errs, fmt.Sprintf("claude backups: %s", err))
	}

	if _, err := r.Output(ctx, "pi", "mcp", "remove", "sag"); err != nil {
		errs = append(errs, fmt.Sprintf("pi mcp remove sag: %s", err))
	}
	if err := os.RemoveAll(filepath.Dir(pi.SkillFile)); err != nil {
		errs = append(errs, fmt.Sprintf("pi skills sag: %s", err))
	}
	if err := os.Remove(pi.GuardFile); err != nil && !os.IsNotExist(err) {
		errs = append(errs, fmt.Sprintf("pi extensions sag-guard.ts: %s", err))
	}

	if profile, hasProfile := agentsetup.ShellProfile(runtime.GOOS, shell, zdot, home); hasProfile {
		if err := removeShellSAGLines(profile, cfgDir); err != nil {
			errs = append(errs, fmt.Sprintf("shell profile sag lines: %s", err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("teardown harness files: %s", strings.Join(errs, "; "))
	}
	return nil
}

func removeClaudeSettingsSAGEntries(path, cfgDir string) error {
	caPath := cfgDir + "/ca.pem"
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
			if raw, has := env["NODE_EXTRA_CA_CERTS"]; has {
				var val string
				if json.Unmarshal(raw, &val) == nil && val == caPath {
					delete(env, "NODE_EXTRA_CA_CERTS")
					changed = true
					if len(env) == 0 {
						delete(top, "env")
					} else {
						b, err := marshalRaw(env)
						if err != nil {
							return err
						}
						top["env"] = b
					}
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
							b, err := marshalRaw(kept)
							if err != nil {
								return err
							}
							perms["deny"] = b
						}
						if len(perms) == 0 {
							delete(top, "permissions")
						} else {
							b, err := marshalRaw(perms)
							if err != nil {
								return err
							}
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
	encoded, err := marshalRaw(top)
	if err != nil {
		return err
	}
	return writeFileSymlinkSafe(path, encoded)
}

func marshalRaw(v interface{}) (json.RawMessage, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	b := buf.Bytes()
	if len(b) > 0 && b[len(b)-1] == '\n' {
		b = b[:len(b)-1]
	}
	return b, nil
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

func removeShellSAGLines(path, cfgDir string) error {
	caLine := `export NODE_EXTRA_CA_CERTS="` + cfgDir + `/ca.pem"`
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	content := string(data)
	lines := strings.Split(content, "\n")
	var kept []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "# sag agent setup" || trimmed == caLine {
			continue
		}
		kept = append(kept, line)
	}
	if len(kept) == len(lines) {
		return nil
	}
	out := strings.Join(kept, "\n")
	return writeFileSymlinkSafe(path, []byte(out))
}

func writeFileSymlinkSafe(linkPath string, data []byte) error {
	targetPath := linkPath
	linfo, lstatErr := os.Lstat(linkPath)
	if lstatErr == nil && linfo.Mode()&os.ModeSymlink != 0 {
		dest, err := os.Readlink(linkPath)
		if err != nil {
			return err
		}
		if !filepath.IsAbs(dest) {
			dest = filepath.Join(filepath.Dir(linkPath), dest)
		}
		targetPath = dest
	}

	var mode os.FileMode = 0o600
	if tinfo, err := os.Stat(targetPath); err == nil {
		mode = tinfo.Mode().Perm()
	}

	tmp, err := os.CreateTemp(filepath.Dir(targetPath), ".sag-tmp-*")
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
	_ = os.Chmod(tmpName, mode)
	return os.Rename(tmpName, targetPath)
}

func UserChildEnv(env []string, realBinDir string) []string {
	var out []string
	for _, e := range env {
		key, _, _ := strings.Cut(e, "=")
		if allowedEnvKeys[key] || strings.HasPrefix(key, "SAG_TEST_") {
			out = append(out, e)
		}
	}
	out = append(out, "PATH="+realBinDir+":"+systemPATH)
	return out
}
