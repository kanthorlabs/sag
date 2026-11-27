package agentsetup

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/golang-jwt/jwt/v5"
	"github.com/kanthorlabs/sag/internal/prompt"
)

func TestURLForms(t *testing.T) {
	cases := []struct {
		url     string
		wantErr bool
	}{
		{"https://example.com:8443/mcp", false},
		{"https://192.168.1.1:8443/mcp", false},
		{"https://100.64.1.2:8443/mcp", false},
		{"http://example.com:8443/mcp", true},
		{"https://example.com:9000/mcp", true},
		{"https://example.com:8443/other", true},
		{"https://:8443/mcp", true},
		{"", true},
		{"not-a-url", true},
		{"https://example.com:8443/mcp/extra", true},
	}
	for _, c := range cases {
		c := c
		t.Run(c.url, func(t *testing.T) {
			mode, err := ParseURL(c.url)
			if c.wantErr {
				if err == nil {
					t.Fatalf("ParseURL(%q): want error, got nil (mode %v)", c.url, mode)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseURL(%q): want nil error, got %v", c.url, err)
			}
			if mode != ModeRemote {
				t.Fatalf("ParseURL(%q): want ModeRemote, got %v", c.url, mode)
			}
		})
	}
}

func TestNoPodmanCall(t *testing.T) {
	podmanDir := t.TempDir()
	podmanScript := filepath.Join(podmanDir, "podman")
	podmanRecord := filepath.Join(podmanDir, "podman_called")
	scriptContent := fmt.Sprintf("#!/bin/sh\ntouch %s\n", podmanRecord)
	if err := os.WriteFile(podmanScript, []byte(scriptContent), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", podmanDir)

	sub := "remote-a"
	jwtStr := remoteTestJWT(t, sub)

	oldOpen := remoteOpenTTY
	defer func() { remoteOpenTTY = oldOpen }()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	remoteOpenTTY = func() (*os.File, error) { return r, nil }

	go func() {
		fmt.Fprintln(w, jwtStr)
		w.Close()
	}()

	opts := Options{
		Harness: HarnessClaudeCode,
		Sub:     sub,
		URL:     "https://127.0.0.1:8443/mcp",
		Home:    t.TempDir(),
		Env:     map[string]string{},
	}
	cred, err := remoteJWT(context.Background(), opts)
	if err != nil {
		t.Fatalf("TestNoPodmanCall: remoteJWT error: %v", err)
	}
	if cred.Name() != "jwt-"+sub {
		t.Fatalf("TestNoPodmanCall: credential name = %q; want jwt-%s", cred.Name(), sub)
	}

	if _, err := os.Stat(podmanRecord); err == nil {
		t.Fatal("TestNoPodmanCall: podman was called in remote mode")
	}
}

func TestSubMismatchExit1(t *testing.T) {
	wantSub := "alice"
	otherSub := "bob"
	jwtStr := remoteTestJWT(t, otherSub)

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	go func() {
		fmt.Fprintln(w, jwtStr)
		w.Close()
	}()

	_, err = ReadJWT(r, wantSub)
	if err == nil {
		t.Fatal("TestSubMismatchExit1: want error for sub mismatch, got nil")
	}
	if !strings.Contains(err.Error(), "bob") || !strings.Contains(err.Error(), "alice") {
		t.Fatalf("TestSubMismatchExit1: error %q does not name both subs", err.Error())
	}

	r2, w2, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r2.Close()
	goodJWT := remoteTestJWT(t, wantSub)
	go func() {
		fmt.Fprintln(w2, goodJWT)
		w2.Close()
	}()
	cred, err := ReadJWT(r2, wantSub)
	if err != nil {
		t.Fatalf("TestSubMismatchExit1 positive: want nil error, got %v", err)
	}
	if cred.Name() != "jwt-"+wantSub {
		t.Fatalf("TestSubMismatchExit1 positive: want name jwt-%s, got %s", wantSub, cred.Name())
	}
}

func TestNoTerminalExit3NoChange(t *testing.T) {
	home := t.TempDir()
	cfgDir := filepath.Join(home, ".config", "sag")
	if err := os.MkdirAll(cfgDir, 0o700); err != nil {
		t.Fatal(err)
	}
	caPath := filepath.Join(cfgDir, "ca.pem")
	if err := os.WriteFile(caPath, []byte("fake-ca"), 0o600); err != nil {
		t.Fatal(err)
	}

	claudeDir := filepath.Join(home, "claude")
	if err := os.MkdirAll(claudeDir, 0o700); err != nil {
		t.Fatal(err)
	}
	mcpFile := filepath.Join(claudeDir, ".claude.json")

	oldOpen := remoteOpenTTY
	defer func() { remoteOpenTTY = oldOpen }()
	remoteOpenTTY = func() (*os.File, error) {
		return nil, errors.New("no terminal")
	}

	opts := Options{
		Harness: HarnessClaudeCode,
		Sub:     "remote-a",
		URL:     "https://127.0.0.1:8443/mcp",
		CfgDir:  cfgDir,
		Home:    home,
		Env:     map[string]string{"CLAUDE_CONFIG_DIR": claudeDir},
	}

	err := planRemote(context.Background(), opts)
	if err == nil {
		t.Fatal("TestNoTerminalExit3NoChange: want error, got nil")
	}
	if !errors.Is(err, prompt.ErrNoTerminal) {
		t.Fatalf("TestNoTerminalExit3NoChange: want ErrNoTerminal, got %v", err)
	}

	if _, statErr := os.Stat(mcpFile); statErr == nil {
		t.Fatal("TestNoTerminalExit3NoChange: MCP file was written but should not have been")
	}
}

func TestVariableHeaderIsOtherForm(t *testing.T) {
	home := t.TempDir()
	cfgDir := filepath.Join(home, ".config", "sag")
	if err := os.MkdirAll(cfgDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "ca.pem"), []byte("fake-ca"), 0o600); err != nil {
		t.Fatal(err)
	}

	claudeDir := filepath.Join(home, "claude")
	if err := os.MkdirAll(claudeDir, 0o700); err != nil {
		t.Fatal(err)
	}
	mcpFile := filepath.Join(claudeDir, ".claude.json")

	entry := map[string]any{
		"mcpServers": map[string]any{
			"sag": map[string]any{
				"type": "http",
				"url":  "https://127.0.0.1:8443/mcp",
				"headers": map[string]string{
					"Authorization": "${SAG_JWT}",
				},
			},
		},
	}
	data, err := json.Marshal(entry)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mcpFile, data, 0o600); err != nil {
		t.Fatal(err)
	}

	opts := Options{
		Harness: HarnessClaudeCode,
		Sub:     "remote-a",
		URL:     "https://127.0.0.1:8443/mcp",
		CfgDir:  cfgDir,
		Home:    home,
		Env:     map[string]string{"CLAUDE_CONFIG_DIR": claudeDir},
	}

	err = planRemote(context.Background(), opts)
	if err == nil {
		t.Fatal("TestVariableHeaderIsOtherForm: want error, got nil")
	}
	if !strings.Contains(err.Error(), "another form") {
		t.Fatalf("TestVariableHeaderIsOtherForm: want 'another form' in error, got %q", err.Error())
	}

	entry2 := map[string]any{
		"mcpServers": map[string]any{
			"sag": map[string]any{
				"type": "http",
				"url":  "https://127.0.0.1:8443/mcp",
				"headers": map[string]string{
					"Authorization": "Bearer some.jwt.here",
				},
			},
		},
	}
	data2, _ := json.Marshal(entry2)
	if err := os.WriteFile(mcpFile, data2, 0o600); err != nil {
		t.Fatal(err)
	}

	conflict := remoteCheckVariableHeader(mcpFile)
	if conflict != "" {
		t.Fatalf("TestVariableHeaderIsOtherForm: bearer entry should not be another form, got %q", conflict)
	}
}

func remoteTestJWT(t *testing.T, sub string) string {
	t.Helper()
	claims := jwt.MapClaims{"sub": sub}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	str, err := tok.SignedString([]byte("test-key"))
	if err != nil {
		t.Fatal(err)
	}
	return str
}
