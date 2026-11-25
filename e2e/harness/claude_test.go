package harness

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const fakeClaudeScript = `#!/bin/sh
if [ "$1" = "auth" ]; then
	echo '{"loggedIn": LOGGED_IN}'
	exit 0
fi
while [ $# -gt 0 ]; do
	if [ "$1" = "--mcp-config" ]; then
		config="$2"
	fi
	shift
done
echo "config=$config"
echo "ca=$NODE_EXTRA_CA_CERTS"
echo "config_mode=$(ls -l "$config" | cut -c1-10)"
echo "ca_mode=$(ls -l "$NODE_EXTRA_CA_CERTS" | cut -c1-10)"
echo "config_json=$(tr -d '\n' < "$config")"
`

func installFakeClaude(t *testing.T, loggedIn bool) {
	t.Helper()
	dir := t.TempDir()
	script := strings.Replace(fakeClaudeScript, "LOGGED_IN", map[bool]string{true: "true", false: "false"}[loggedIn], 1)
	if err := os.WriteFile(filepath.Join(dir, "claude"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+"/bin"+string(os.PathListSeparator)+"/usr/bin")
}

func fakeClaudeRequest() ClaudeRequest {
	return ClaudeRequest{
		MCPURL:       "https://localhost:1/mcp",
		Token:        "eyJ.test.token",
		CAPEM:        []byte("-----BEGIN CERTIFICATE-----\nAA==\n-----END CERTIFICATE-----\n"),
		Prompt:       "find the marker",
		AllowedTools: []string{"mcp__sag__list_resources", "mcp__sag__sql_query"},
	}
}

func fakeClaudeFields(t *testing.T, stdout string) map[string]string {
	t.Helper()
	fields := map[string]string{}
	for _, line := range strings.Split(stdout, "\n") {
		if k, v, ok := strings.Cut(line, "="); ok {
			fields[k] = v
		}
	}
	return fields
}

func TestConfigFileMode0600(t *testing.T) {
	installFakeClaude(t, true)
	res, err := RunClaude(context.Background(), fakeClaudeRequest())
	if err != nil {
		t.Fatalf("RunClaude: %v", err)
	}
	fields := fakeClaudeFields(t, res.Stdout)
	for _, key := range []string{"config_mode", "ca_mode"} {
		if fields[key] != "-rw-------" {
			t.Fatalf("%s = %q; want -rw-------\n%s", key, fields[key], res.Stdout)
		}
	}
	var cfg struct {
		MCPServers map[string]struct {
			Type    string            `json:"type"`
			URL     string            `json:"url"`
			Headers map[string]string `json:"headers"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal([]byte(fields["config_json"]), &cfg); err != nil {
		t.Fatalf("config JSON: %v", err)
	}
	sag, ok := cfg.MCPServers["sag"]
	if !ok || len(cfg.MCPServers) != 1 {
		t.Fatalf("mcpServers = %v; want only sag", cfg.MCPServers)
	}
	if sag.Type != "http" || sag.URL != "https://localhost:1/mcp" || sag.Headers["Authorization"] != "Bearer eyJ.test.token" {
		t.Fatalf("sag server = %+v; want http, the MCP URL and the bearer header", sag)
	}
}

func TestConfigFileRemoved(t *testing.T) {
	installFakeClaude(t, true)
	res, err := RunClaude(context.Background(), fakeClaudeRequest())
	if err != nil {
		t.Fatalf("RunClaude: %v", err)
	}
	fields := fakeClaudeFields(t, res.Stdout)
	for _, key := range []string{"config", "ca"} {
		path := fields[key]
		if path == "" {
			t.Fatalf("%s path is empty\n%s", key, res.Stdout)
		}
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("%s file %s exists after return: %v", key, path, err)
		}
		if _, err := os.Stat(filepath.Dir(path)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("%s directory %s exists after return: %v", key, filepath.Dir(path), err)
		}
	}
}

func TestClaudePrerequisiteMissing(t *testing.T) {
	t.Run("not in PATH", func(t *testing.T) {
		t.Setenv("PATH", t.TempDir())
		if _, err := RunClaude(context.Background(), fakeClaudeRequest()); !errors.Is(err, ErrClaudePrerequisite) {
			t.Fatalf("err = %v; want ErrClaudePrerequisite", err)
		}
	})
	t.Run("not logged in", func(t *testing.T) {
		installFakeClaude(t, false)
		if _, err := RunClaude(context.Background(), fakeClaudeRequest()); !errors.Is(err, ErrClaudePrerequisite) {
			t.Fatalf("err = %v; want ErrClaudePrerequisite", err)
		}
	})
}
