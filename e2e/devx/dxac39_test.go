//go:build devxe2e

package devx_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/kanthorlabs/sag/e2e/devx/dxh"
)

func init() {
	agentTests["DXAC39_ClaudeCodeSetup"] = func(t *testing.T, ctx context.Context) {
		DXAC39_ClaudeCodeSetup(t, ctx)
	}
}

func DXAC39_ClaudeCodeSetup(t *testing.T, ctx context.Context) {
	t.Helper()
	dxh.EnsureBootstrapped(t)

	sagBin, err := dxh.TestBinary()
	if err != nil {
		t.Fatalf("DXAC39: test binary: %v", err)
	}

	groupDir := t.TempDir()
	binDir := t.TempDir()

	env, err := dxh.ChildEnv(os.Environ(), groupDir)
	if err != nil {
		t.Fatalf("DXAC39: child env: %v", err)
	}

	claudeDir := dxac39FindClaudeDir(env)

	home := os.Getenv("HOME")
	caPath := filepath.Join(dxh.CfgDir(home), "ca.pem")

	fakeClaude := filepath.Join(binDir, "claude")
	fakePodman := filepath.Join(binDir, "podman")

	claudeFakeContent := "#!/bin/sh\n" +
		"if [ \"$1\" = mcp ] && [ \"$2\" = list ]; then\n" +
		"  echo 'sag: https://localhost:8443/mcp (HTTP) - Connected'\n" +
		"  exit 0\n" +
		"fi\n" +
		"exit 1\n"
	if err := os.WriteFile(fakeClaude, []byte(claudeFakeContent), 0o755); err != nil {
		t.Fatalf("DXAC39: write fake claude: %v", err)
	}

	podmanFakeContent := "#!/bin/sh\n" +
		"if [ \"$1\" = exec ] && [ \"$2\" = sag ] && [ \"$3\" = sag ] && [ \"$4\" = agent ] && [ \"$5\" = issue ]; then\n" +
		"  sub=\"$6\"\n" +
		"  " + sagBin + " agent issue \"$sub\"\n" +
		"  exit $?\n" +
		"fi\n" +
		"exec /usr/local/bin/podman \"$@\" 2>/dev/null || exec /usr/bin/podman \"$@\"\n"
	podmanBin := dxac39RealPodman()
	if podmanBin != "" {
		podmanFakeContent = "#!/bin/sh\n" +
			"if [ \"$1\" = exec ] && [ \"$2\" = sag ] && [ \"$3\" = sag ] && [ \"$4\" = agent ] && [ \"$5\" = issue ]; then\n" +
			"  sub=\"$6\"\n" +
			"  " + podmanBin + " exec sag sag agent issue \"$sub\"\n" +
			"  exit $?\n" +
			"fi\n" +
			"exec " + podmanBin + " \"$@\"\n"
	}
	if err := os.WriteFile(fakePodman, []byte(podmanFakeContent), 0o755); err != nil {
		t.Fatalf("DXAC39: write fake podman: %v", err)
	}

	env = append(env, "PATH="+dxh.TestPATH(binDir))
	env = append(env, "HOME="+home)

	testCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(testCtx, sagBin, "agent", "setup", "claude-code")
	cmd.Env = env
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	out, runErr := cmd.CombinedOutput()
	if runErr != nil {
		t.Fatalf("DXAC39: sag agent setup claude-code: %v\noutput:\n%s", runErr, out)
	}

	t.Run("mcp_entry_in_config", func(t *testing.T) {
		mcpFile := filepath.Join(claudeDir, ".claude.json")
		data, err := os.ReadFile(mcpFile)
		if err != nil {
			t.Fatalf("DXAC39-A: read %s: %v", mcpFile, err)
		}
		var top map[string]json.RawMessage
		if err := json.Unmarshal(data, &top); err != nil {
			t.Fatalf("DXAC39-A: unmarshal mcp file: %v", err)
		}
		var servers map[string]json.RawMessage
		if err := json.Unmarshal(top["mcpServers"], &servers); err != nil {
			t.Fatalf("DXAC39-A: unmarshal mcpServers: %v", err)
		}
		sagRaw, ok := servers["sag"]
		if !ok {
			t.Fatal("DXAC39-A: mcpServers.sag absent")
		}
		var entry map[string]json.RawMessage
		if err := json.Unmarshal(sagRaw, &entry); err != nil {
			t.Fatalf("DXAC39-A: unmarshal sag entry: %v", err)
		}
		if _, hasType := entry["type"]; !hasType {
			t.Error("DXAC39-A: sag entry missing 'type'")
		}
		var entryType string
		if err := json.Unmarshal(entry["type"], &entryType); err != nil || entryType != "http" {
			t.Errorf("DXAC39-A: sag entry type = %q; want http", entryType)
		}
		var headers map[string]string
		if err := json.Unmarshal(entry["headers"], &headers); err != nil {
			t.Fatalf("DXAC39-A: unmarshal headers: %v", err)
		}
		if !strings.HasPrefix(headers["Authorization"], "Bearer ") {
			t.Errorf("DXAC39-A: Authorization header = %q; want Bearer ...", headers["Authorization"])
		}
		if strings.Contains(headers["Authorization"], "\n") || strings.Contains(headers["Authorization"], " ") {
			parts := strings.Fields(headers["Authorization"])
			if len(parts) != 2 || parts[0] != "Bearer" {
				t.Errorf("DXAC39-A: malformed Authorization header: %q", headers["Authorization"])
			}
		}
	})

	t.Run("skill_file_has_marker", func(t *testing.T) {
		skillFile := filepath.Join(claudeDir, "skills", "sag", "SKILL.md")
		data, err := os.ReadFile(skillFile)
		if err != nil {
			t.Fatalf("DXAC39-B: read skill file: %v", err)
		}
		if !strings.Contains(string(data), "<!-- managed by sag agent setup -->") {
			t.Error("DXAC39-B: skill file missing managed marker")
		}
	})

	t.Run("settings_has_ca_trust", func(t *testing.T) {
		settingsFile := filepath.Join(claudeDir, "settings.json")
		data, err := os.ReadFile(settingsFile)
		if err != nil {
			t.Fatalf("DXAC39-C: read settings.json: %v", err)
		}
		var settings map[string]json.RawMessage
		if err := json.Unmarshal(data, &settings); err != nil {
			t.Fatalf("DXAC39-C: unmarshal settings.json: %v", err)
		}
		envRaw, ok := settings["env"]
		if !ok {
			t.Fatal("DXAC39-C: settings.json missing env key")
		}
		var envMap map[string]string
		if err := json.Unmarshal(envRaw, &envMap); err != nil {
			t.Fatalf("DXAC39-C: unmarshal env: %v", err)
		}
		necc := envMap["NODE_EXTRA_CA_CERTS"]
		if necc != caPath {
			t.Errorf("DXAC39-C: NODE_EXTRA_CA_CERTS = %q; want %q", necc, caPath)
		}
	})

	t.Run("output_has_ok_line", func(t *testing.T) {
		outStr := string(out)
		if !strings.Contains(outStr, "ok") {
			t.Errorf("DXAC39-D: output missing 'ok' line; got:\n%s", outStr)
		}
	})
}

func dxac39FindClaudeDir(env []string) string {
	for _, e := range env {
		if strings.HasPrefix(e, "CLAUDE_CONFIG_DIR=") {
			return strings.TrimPrefix(e, "CLAUDE_CONFIG_DIR=")
		}
	}
	return ""
}

func dxac39RealPodman() string {
	for _, p := range []string{"/usr/local/bin/podman", "/usr/bin/podman"} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}
