//go:build devxe2e

package devx_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"

	"github.com/kanthorlabs/sag/e2e/devx/dxh"
)

func dxac26CompactJSON(raw json.RawMessage) []byte {
	var buf bytes.Buffer
	if err := json.Compact(&buf, raw); err != nil {
		return raw
	}
	return buf.Bytes()
}

func init() {
	agentTests["DXAC26_DenyRules"] = func(t *testing.T, ctx context.Context) {
		DXAC26_DenyRules(t, ctx)
	}
}

func DXAC26_DenyRules(t *testing.T, ctx context.Context) {
	t.Helper()
	dxh.EnsureBootstrapped(t)

	bin, err := dxh.TestBinary()
	if err != nil {
		t.Fatalf("DXAC26: test binary: %v", err)
	}
	podmanPath, err := exec.LookPath("podman")
	if err != nil {
		t.Fatalf("DXAC26: find podman: %v", err)
	}

	t.Run("adds_three_rules", func(t *testing.T) {
		claudeDir := t.TempDir()
		binDir := t.TempDir()
		if err := dxh.AddRealProgramLink(binDir, "podman", podmanPath); err != nil {
			t.Fatal(err)
		}
		settings := filepath.Join(claudeDir, "settings.json")
		initial := `{"other":"value","permissions":{"allow":["Bash(ls:*)"]}}`
		if err := os.WriteFile(settings, []byte(initial), 0o640); err != nil {
			t.Fatalf("DXAC26: write settings: %v", err)
		}

		env := dxac26Env(t, claudeDir, binDir)
		code, out := dxac26RunSetsid(ctx, bin, env, "agent", "setup", "claude-code", "--deny-rules", "yes")
		if code != 0 {
			t.Fatalf("DXAC26: agent setup claude-code --deny-rules yes: exit %d\n%s", code, out)
		}

		dxac26AssertDenyRules(t, settings)

		var origTop map[string]json.RawMessage
		if err := json.Unmarshal([]byte(initial), &origTop); err != nil {
			t.Fatalf("DXAC26: parse initial: %v", err)
		}
		var top map[string]json.RawMessage
		if err := json.Unmarshal(dxac26ReadFile(t, settings), &top); err != nil {
			t.Fatalf("DXAC26: parse settings: %v", err)
		}
		if _, ok := top["other"]; !ok {
			t.Error("DXAC26: 'other' key was removed from settings.json")
		}
		if !bytes.Equal(dxac26CompactJSON(origTop["other"]), dxac26CompactJSON(top["other"])) {
			t.Errorf("DXAC26: 'other' value changed: got %s; want %s",
				dxac26CompactJSON(top["other"]), dxac26CompactJSON(origTop["other"]))
		}
		if origAllow, ok := origTop["permissions"]; ok {
			var origPerm, gotPerm struct {
				Allow []string `json:"allow"`
			}
			json.Unmarshal(origAllow, &origPerm)
			json.Unmarshal(top["permissions"], &gotPerm)
			for _, rule := range origPerm.Allow {
				found := false
				for _, r := range gotPerm.Allow {
					if r == rule {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("DXAC26: original allow rule %q missing from settings.json", rule)
				}
			}
		}
	})

	t.Run("no_duplicate_on_second_run", func(t *testing.T) {
		claudeDir := t.TempDir()
		binDir := t.TempDir()
		if err := dxh.AddRealProgramLink(binDir, "podman", podmanPath); err != nil {
			t.Fatal(err)
		}

		env := dxac26Env(t, claudeDir, binDir)
		for i := 0; i < 2; i++ {
			code, out := dxac26RunSetsid(ctx, bin, env, "agent", "setup", "claude-code", "--deny-rules", "yes")
			if code != 0 {
				t.Fatalf("DXAC26: run %d: exit %d\n%s", i+1, code, out)
			}
		}
		settings := filepath.Join(claudeDir, "settings.json")
		rules := dxac26ReadDenyRules(t, settings)
		if count := countOccurrences(rules, "Bash(sag credential delete:*)"); count != 1 {
			t.Errorf("DXAC26: 'Bash(sag credential delete:*)' appears %d times; want 1", count)
		}
		if count := countOccurrences(rules, "Bash(podman secret rm:*)"); count != 1 {
			t.Errorf("DXAC26: 'Bash(podman secret rm:*)' appears %d times; want 1", count)
		}
		if count := countOccurrences(rules, "Bash(script:*)"); count != 1 {
			t.Errorf("DXAC26: 'Bash(script:*)' appears %d times; want 1", count)
		}
	})

	t.Run("deny_rules_no_leaves_unchanged", func(t *testing.T) {
		claudeDir := t.TempDir()
		binDir := t.TempDir()
		if err := dxh.AddRealProgramLink(binDir, "podman", podmanPath); err != nil {
			t.Fatal(err)
		}
		settings := filepath.Join(claudeDir, "settings.json")
		initial := `{"other":"kept","permissions":{"deny":["Bash(sag credential delete:*)","Bash(podman secret rm:*)","Bash(script:*)","Bash(other-rule:*)"]}}`
		if err := os.WriteFile(settings, []byte(initial), 0o600); err != nil {
			t.Fatalf("DXAC26: write settings: %v", err)
		}

		env := dxac26Env(t, claudeDir, binDir)
		code, out := dxac26RunSetsid(ctx, bin, env, "agent", "setup", "claude-code", "--deny-rules", "no")
		if code != 0 {
			t.Fatalf("DXAC26: agent setup --deny-rules no: exit %d\n%s", code, out)
		}

		data, err := os.ReadFile(settings)
		if err != nil {
			t.Fatalf("DXAC26: read settings: %v", err)
		}
		var got map[string]json.RawMessage
		if err := json.Unmarshal(data, &got); err != nil {
			t.Fatalf("DXAC26: parse settings after --deny-rules no: %v", err)
		}
		var orig map[string]json.RawMessage
		if err := json.Unmarshal([]byte(initial), &orig); err != nil {
			t.Fatalf("DXAC26: parse initial: %v", err)
		}
		if !bytes.Equal(dxac26CompactJSON(orig["permissions"]), dxac26CompactJSON(got["permissions"])) {
			t.Errorf("DXAC26: permissions changed after --deny-rules no; got %s; want %s",
				dxac26CompactJSON(got["permissions"]), dxac26CompactJSON(orig["permissions"]))
		}
	})

	t.Run("deny_rules_no_adds_no_deny_key", func(t *testing.T) {
		claudeDir := t.TempDir()
		binDir := t.TempDir()
		if err := dxh.AddRealProgramLink(binDir, "podman", podmanPath); err != nil {
			t.Fatal(err)
		}
		settings := filepath.Join(claudeDir, "settings.json")
		if err := os.WriteFile(settings, []byte(`{"other":"kept"}`), 0o600); err != nil {
			t.Fatalf("DXAC26: write settings: %v", err)
		}

		env := dxac26Env(t, claudeDir, binDir)
		code, out := dxac26RunSetsid(ctx, bin, env, "agent", "setup", "claude-code", "--deny-rules", "no")
		if code != 0 {
			t.Fatalf("DXAC26: agent setup --deny-rules no: exit %d\n%s", code, out)
		}

		rules := dxac26ReadDenyRules(t, settings)
		if len(rules) != 0 {
			t.Errorf("DXAC26: deny rules added when answer was 'no'; got %v", rules)
		}
	})
}

func dxac26Env(t *testing.T, claudeDir, binDir string) []string {
	t.Helper()
	if fakeBinDir := dxh.ActiveAgentBinDir; fakeBinDir != "" {
		if err := dxh.AddRealProgramLink(binDir, "claude", filepath.Join(fakeBinDir, "claude")); err != nil {
			t.Fatalf("DXAC26: link claude stub: %v", err)
		}
	}
	var env []string
	for _, e := range os.Environ() {
		key, _, _ := strings.Cut(e, "=")
		switch key {
		case "HOME", "TMPDIR", "USER", "LANG":
			env = append(env, e)
		default:
			if strings.HasPrefix(key, "SAG_TEST_") {
				env = append(env, e)
			}
		}
	}
	env = append(env, "CLAUDE_CONFIG_DIR="+claudeDir)
	env = append(env, "PI_CODING_AGENT_DIR="+t.TempDir())
	env = append(env, "PATH="+dxh.TestPATH(binDir))
	return env
}

func dxac26RunSetsid(ctx context.Context, bin string, env []string, args ...string) (int, string) {
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = env
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	runErr := cmd.Run()
	code := 0
	if runErr != nil {
		if exitErr, ok := runErr.(*exec.ExitError); ok {
			code = exitErr.ExitCode()
		}
	}
	return code, out.String()
}

func dxac26AssertDenyRules(t *testing.T, settingsPath string) {
	t.Helper()
	rules := dxac26ReadDenyRules(t, settingsPath)
	want := []string{"Bash(sag credential delete:*)", "Bash(podman secret rm:*)", "Bash(script:*)"}
	for _, rule := range want {
		if !slices.Contains(rules, rule) {
			t.Errorf("DXAC26: deny rule %q not found in settings.json; rules: %v", rule, rules)
		}
	}
}

func dxac26ReadDenyRules(t *testing.T, settingsPath string) []string {
	t.Helper()
	var top struct {
		Permissions struct {
			Deny []string `json:"deny"`
		} `json:"permissions"`
	}
	data := dxac26ReadFile(t, settingsPath)
	if err := json.Unmarshal(data, &top); err != nil {
		t.Fatalf("DXAC26: parse settings.json: %v", err)
	}
	return top.Permissions.Deny
}

func dxac26ReadFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("DXAC26: read %s: %v", path, err)
	}
	return data
}

func countOccurrences(ss []string, s string) int {
	count := 0
	for _, v := range ss {
		if v == s {
			count++
		}
	}
	return count
}
