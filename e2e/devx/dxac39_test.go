//go:build devxe2e

package devx_test

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
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
	harnessDir := dxac39BuildFakeHarness(t, binDir)

	env, err := dxh.ChildEnv(os.Environ(), groupDir)
	if err != nil {
		t.Fatalf("DXAC39: child env: %v", err)
	}
	claudeDir := dxac39FindVal(env, "CLAUDE_CONFIG_DIR")
	if claudeDir == "" {
		t.Fatal("DXAC39: CLAUDE_CONFIG_DIR not set in child env")
	}
	env = append(env, "PATH="+dxh.TestPATH(binDir))
	home := os.Getenv("HOME")
	if home != "" {
		env = append(env, "HOME="+home)
	}

	sagCfgDir := dxh.CfgDir(home)
	env = append(env, "NODE_EXTRA_CA_CERTS="+filepath.Join(sagCfgDir, "ca.pem"))

	if err := os.MkdirAll(claudeDir, 0o700); err != nil {
		t.Fatalf("DXAC39: mkdir claude dir: %v", err)
	}

	mcpFile := filepath.Join(claudeDir, ".claude.json")
	initMCP := `{"mcpServers":{"other-server":{"type":"http","url":"https://other.example/mcp","headers":{}}},"topInt":9007199254740993,"topUnicode":"\u0041nother","topSpecial":"a<b>&c"}`
	if err := os.WriteFile(mcpFile, []byte(initMCP), 0o644); err != nil {
		t.Fatalf("DXAC39: write initial .claude.json: %v", err)
	}

	settingsTarget := filepath.Join(claudeDir, "settings_target.json")
	initSettings := `{"other-setting":true}`
	if err := os.WriteFile(settingsTarget, []byte(initSettings), 0o640); err != nil {
		t.Fatalf("DXAC39: write settings target: %v", err)
	}
	settingsLink := filepath.Join(claudeDir, "settings.json")
	if err := os.Symlink(settingsTarget, settingsLink); err != nil {
		t.Fatalf("DXAC39: symlink settings.json: %v", err)
	}

	testCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(testCtx, sagBin, "agent", "setup", "claude-code", "--deny-rules", "no")
	cmd.Env = env
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	runErr := cmd.Run()
	exitCode := 0
	if runErr != nil {
		if exitErr, ok := runErr.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			t.Fatalf("DXAC39: run: %v", runErr)
		}
	}

	t.Run("exit_0", func(t *testing.T) {
		if exitCode != 0 {
			t.Errorf("DXAC39-exit: exit = %d; want 0; stderr=%s stdout=%s",
				exitCode, errBuf.String(), outBuf.String())
		}
	})

	if exitCode != 0 {
		t.Fatalf("DXAC39: setup failed (exit %d); cannot run further assertions", exitCode)
	}

	t.Run("mcp_file_mode_0600", func(t *testing.T) {
		fi, err := os.Lstat(mcpFile)
		if err != nil {
			t.Fatalf("DXAC39-A: stat .claude.json: %v", err)
		}
		if fi.Mode()&0o777 != 0o600 {
			t.Errorf("DXAC39-A: .claude.json mode = %o; want 0600", fi.Mode()&0o777)
		}
	})

	mcpData, err := os.ReadFile(mcpFile)
	if err != nil {
		t.Fatalf("DXAC39: read .claude.json: %v", err)
	}
	var mcpTop map[string]json.RawMessage
	if err := json.Unmarshal(mcpData, &mcpTop); err != nil {
		t.Fatalf("DXAC39: unmarshal .claude.json: %v", err)
	}
	var servers map[string]json.RawMessage
	if err := json.Unmarshal(mcpTop["mcpServers"], &servers); err != nil {
		t.Fatalf("DXAC39: unmarshal mcpServers: %v", err)
	}

	var sagEntry map[string]json.RawMessage
	t.Run("sag_entry_exists", func(t *testing.T) {
		sagRaw, ok := servers["sag"]
		if !ok {
			t.Fatal("DXAC39-B: mcpServers.sag absent")
		}
		if err := json.Unmarshal(sagRaw, &sagEntry); err != nil {
			t.Fatalf("DXAC39-B: unmarshal sag entry: %v", err)
		}
	})

	var jwtStr string
	t.Run("sag_entry_fields", func(t *testing.T) {
		if sagEntry == nil {
			t.Fatal("DXAC39-C: sag entry missing")
		}
		if _, ok := sagEntry["command"]; ok {
			t.Error("DXAC39-C: sag entry has 'command' key; want none")
		}
		var entryType string
		if err := json.Unmarshal(sagEntry["type"], &entryType); err != nil || entryType != "http" {
			t.Errorf("DXAC39-C: sag entry type = %q; want http", entryType)
		}
		var entryURL string
		if err := json.Unmarshal(sagEntry["url"], &entryURL); err != nil || entryURL != "https://localhost:8443/mcp" {
			t.Errorf("DXAC39-C: sag entry url = %q; want https://localhost:8443/mcp", entryURL)
		}
		var headers map[string]string
		if err := json.Unmarshal(sagEntry["headers"], &headers); err != nil {
			t.Fatalf("DXAC39-C: unmarshal headers: %v", err)
		}
		auth := headers["Authorization"]
		if !strings.HasPrefix(auth, "Bearer ") {
			t.Errorf("DXAC39-C: Authorization header missing Bearer prefix")
		} else {
			jwtStr = strings.TrimPrefix(auth, "Bearer ")
		}
	})

	t.Run("existing_values_preserved", func(t *testing.T) {
		otherRaw, ok := servers["other-server"]
		if !ok {
			t.Error("DXAC39-D: other-server entry removed")
		} else {
			var wantCompact bytes.Buffer
			json.Compact(&wantCompact, []byte(`{"type":"http","url":"https://other.example/mcp","headers":{}}`))
			var gotCompact bytes.Buffer
			json.Compact(&gotCompact, otherRaw)
			if wantCompact.String() != gotCompact.String() {
				t.Errorf("DXAC39-D: other-server = %s; want %s", gotCompact.String(), wantCompact.String())
			}
		}
		dxac39CheckRaw(t, "DXAC39-D topInt", mcpTop["topInt"], `9007199254740993`)
		dxac39CheckRaw(t, "DXAC39-D topUnicode", mcpTop["topUnicode"], `"\u0041nother"`)
		dxac39CheckRaw(t, "DXAC39-D topSpecial", mcpTop["topSpecial"], `"a<b>&c"`)
	})

	t.Run("settings_symlink_and_ca_trust", func(t *testing.T) {
		fi, err := os.Lstat(settingsLink)
		if err != nil {
			t.Fatalf("DXAC39-E: stat settings.json: %v", err)
		}
		if fi.Mode()&os.ModeSymlink == 0 {
			t.Error("DXAC39-E: settings.json is not a symlink anymore")
		}
		targetFi, err := os.Stat(settingsTarget)
		if err != nil {
			t.Fatalf("DXAC39-E: stat settings target: %v", err)
		}
		if targetFi.Mode()&0o777 != 0o640 {
			t.Errorf("DXAC39-E: settings target mode = %o; want 0640", targetFi.Mode()&0o777)
		}
		data, err := os.ReadFile(settingsTarget)
		if err != nil {
			t.Fatalf("DXAC39-E: read settings target: %v", err)
		}
		var settings map[string]json.RawMessage
		if err := json.Unmarshal(data, &settings); err != nil {
			t.Fatalf("DXAC39-E: unmarshal settings: %v", err)
		}
		envRaw, ok := settings["env"]
		if !ok {
			t.Fatal("DXAC39-E: settings missing env key")
		}
		var envMap map[string]string
		if err := json.Unmarshal(envRaw, &envMap); err != nil {
			t.Fatalf("DXAC39-E: unmarshal env: %v", err)
		}
		wantCA := filepath.Join(sagCfgDir, "ca.pem")
		if necc := envMap["NODE_EXTRA_CA_CERTS"]; necc != wantCA {
			t.Errorf("DXAC39-E: NODE_EXTRA_CA_CERTS = %q; want %q", necc, wantCA)
		}
	})

	t.Run("skill_marker_and_sub", func(t *testing.T) {
		skillFile := filepath.Join(claudeDir, "skills", "sag", "SKILL.md")
		data, err := os.ReadFile(skillFile)
		if err != nil {
			t.Fatalf("DXAC39-F: read skill file: %v", err)
		}
		content := string(data)
		if !strings.Contains(content, "<!-- managed by sag agent setup -->") {
			t.Error("DXAC39-F: skill file missing managed marker")
		}
		if !strings.Contains(content, "--agent claude-code") {
			t.Error("DXAC39-F: skill file missing --agent claude-code")
		}
	})

	t.Run("jwt_authenticates_at_mcp", func(t *testing.T) {
		if jwtStr == "" {
			t.Fatal("DXAC39-G: JWT not extracted")
		}
		caPEM, err := os.ReadFile(filepath.Join(sagCfgDir, "ca.pem"))
		if err != nil {
			t.Fatalf("DXAC39-G: read ca.pem: %v", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(caPEM) {
			t.Fatal("DXAC39-G: ca.pem: no certificate")
		}
		client := &http.Client{
			Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool}},
			Timeout:   10 * time.Second,
		}
		body := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"list_resources","arguments":{}}}`
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://localhost:8443/mcp",
			strings.NewReader(body))
		if err != nil {
			t.Fatalf("DXAC39-G: build request: %v", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+jwtStr)
		req.Header.Set("Accept", "application/json, text/event-stream")
		req.Header.Set("Mcp-Protocol-Version", "2025-06-18")
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("DXAC39-G: POST /mcp: %v", err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("DXAC39-G: POST /mcp status = %d; want 200", resp.StatusCode)
		}

		p := jwt.NewParser(jwt.WithoutClaimsValidation())
		var claims jwt.MapClaims
		_, _, err = p.ParseUnverified(jwtStr, &claims)
		if err != nil {
			t.Fatalf("DXAC39-G: parse JWT: %v", err)
		}
		sub, _ := claims["sub"].(string)
		if sub != "claude-code" {
			t.Errorf("DXAC39-G: JWT sub = %q; want claude-code", sub)
		}
	})

	t.Run("claude_stub_received_mcp_list_in_empty_dir_no_necc", func(t *testing.T) {
		recs := dxac39ReadRecords(t, harnessDir)
		var found bool
		for _, rec := range recs {
			if len(rec.Argv) == 2 && rec.Argv[0] == "mcp" && rec.Argv[1] == "list" {
				found = true
				if necc := rec.Env["NODE_EXTRA_CA_CERTS"]; necc != "" {
					t.Errorf("DXAC39-H: claude mcp list had NODE_EXTRA_CA_CERTS=%q; want absent", necc)
				}
				if rec.Dir == claudeDir || rec.Dir == home {
					t.Errorf("DXAC39-H: claude ran in %q; want an empty temp dir", rec.Dir)
				}
				if len(rec.DirEntries) != 0 {
					t.Errorf("DXAC39-H: claude ran in dir with entries %v; want empty", rec.DirEntries)
				}
				break
			}
		}
		if !found {
			t.Error("DXAC39-H: no 'claude mcp list' call recorded in stub_records.json")
		}
	})
}

type dxac39Record struct {
	Argv       []string          `json:"argv"`
	Env        map[string]string `json:"env"`
	Dir        string            `json:"dir"`
	DirEntries []string          `json:"dir_entries"`
}

func dxac39ReadRecords(t *testing.T, harnessDir string) []dxac39Record {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(harnessDir, "stub_records.json"))
	if err != nil {
		t.Fatalf("DXAC39: read stub_records.json: %v", err)
	}
	var recs []dxac39Record
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		line := sc.Bytes()
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var rec dxac39Record
		if err := json.Unmarshal(line, &rec); err != nil {
			t.Logf("DXAC39: skip malformed record: %v", err)
			continue
		}
		recs = append(recs, rec)
	}
	return recs
}

func dxac39FindVal(env []string, key string) string {
	prefix := key + "="
	for _, e := range env {
		if strings.HasPrefix(e, prefix) {
			return strings.TrimPrefix(e, prefix)
		}
	}
	return ""
}

func dxac39CheckRaw(t *testing.T, label string, got json.RawMessage, want string) {
	t.Helper()
	var gotBuf, wantBuf bytes.Buffer
	json.Compact(&gotBuf, got)
	json.Compact(&wantBuf, []byte(want))
	if gotBuf.String() != wantBuf.String() {
		t.Errorf("%s = %s; want %s", label, gotBuf.String(), wantBuf.String())
	}
}

func dxac39BuildFakeHarness(t *testing.T, binDir string) (harnessDir string) {
	t.Helper()
	root, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		t.Fatalf("DXAC39: git show-toplevel: %v", err)
	}
	pkg := filepath.Join(strings.TrimSpace(string(root)), "e2e", "devx", "cmd", "fakeharness")
	harnessDir = t.TempDir()
	bin := filepath.Join(harnessDir, "fakeharness")
	out, buildErr := exec.Command("go", "build", "-o", bin, pkg).CombinedOutput()
	if buildErr != nil {
		t.Fatalf("DXAC39: build fakeharness: %v\n%s", buildErr, out)
	}
	for _, name := range []string{"claude", "pi"} {
		link := filepath.Join(binDir, name)
		if err := os.Symlink(bin, link); err != nil {
			t.Fatalf("DXAC39: symlink %s: %v", name, err)
		}
	}
	return harnessDir
}

func dxac39MCPClient(t *testing.T, ctx context.Context, sagCfgDir, jwtStr string) ([]string, error) {
	t.Helper()
	caPEM, err := os.ReadFile(filepath.Join(sagCfgDir, "ca.pem"))
	if err != nil {
		return nil, fmt.Errorf("read ca.pem: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		return nil, fmt.Errorf("ca.pem: no certificate")
	}
	client := &http.Client{
		Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool}},
		Timeout:   10 * time.Second,
	}
	body := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"list_resources","arguments":{}}}`
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://localhost:8443/mcp",
		strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+jwtStr)
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Mcp-Protocol-Version", "2025-06-18")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("POST /mcp: status %d", resp.StatusCode)
	}
	var result struct {
		Result struct {
			StructuredContent struct {
				Resources []struct {
					Name string `json:"name"`
				} `json:"resources"`
			} `json:"structuredContent"`
		} `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	names := make([]string, 0, len(result.Result.StructuredContent.Resources))
	for _, r := range result.Result.StructuredContent.Resources {
		names = append(names, r.Name)
	}
	return names, nil
}
