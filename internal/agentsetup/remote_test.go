package agentsetup

import (
	"context"
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
		{"https://user@example.com:8443/mcp", true},
		{"https://example.com:8443/mcp?q=1", true},
		{"https://example.com:8443/mcp#frag", true},
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
	podmanRecord := filepath.Join(podmanDir, "podman_called")

	sub := "remote-a"
	jwtStr := remoteTestJWT(t, sub)

	oldOpen := remoteOpenTTY
	defer func() { remoteOpenTTY = oldOpen }()

	pr, pw, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer pr.Close()
	remoteOpenTTY = func() (*os.File, error) { return pr, nil }

	go func() {
		fmt.Fprintln(pw, jwtStr)
		pw.Close()
	}()

	opts := Options{
		Harness:    HarnessClaudeCode,
		Sub:        sub,
		URL:        "https://127.0.0.1:8443/mcp",
		Home:       t.TempDir(),
		PodmanPath: filepath.Join(podmanDir, "podman"),
		Env:        map[string]string{},
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
		Harness:    HarnessClaudeCode,
		Sub:        "remote-a",
		URL:        "https://127.0.0.1:8443/mcp",
		CfgDir:     cfgDir,
		Home:       home,
		Env:        map[string]string{"CLAUDE_CONFIG_DIR": claudeDir},
		PodmanPath: "podman",
	}

	_, err := planOnly(context.Background(), opts)
	if err == nil {
		t.Fatal("TestNoTerminalExit3NoChange: want error, got nil")
	}
	var refusal *prompt.RefusalError
	if !errors.As(err, &refusal) {
		t.Fatalf("TestNoTerminalExit3NoChange: want *prompt.RefusalError, got %T: %v", err, err)
	}

	if _, statErr := os.Stat(mcpFile); statErr == nil {
		t.Fatal("TestNoTerminalExit3NoChange: MCP file was written but should not have been")
	}
}

func TestVariableHeaderIsOtherForm(t *testing.T) {
	dir := t.TempDir()
	mcpFile := filepath.Join(dir, ".claude.json")

	variableEntry := map[string]any{
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
	writeJSONToFile(t, mcpFile, variableEntry)

	conflict := checkMCPFile(mcpFile, "https://127.0.0.1:8443/mcp")
	if conflict == "" {
		t.Fatal("TestVariableHeaderIsOtherForm: want conflict for variable header, got empty")
	}
	if !strings.Contains(conflict, "another form") {
		t.Fatalf("TestVariableHeaderIsOtherForm: want 'another form' in conflict, got %q", conflict)
	}

	bearerEntry := map[string]any{
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
	writeJSONToFile(t, mcpFile, bearerEntry)

	conflict2 := checkMCPFile(mcpFile, "https://127.0.0.1:8443/mcp")
	if conflict2 != "" {
		t.Fatalf("TestVariableHeaderIsOtherForm: bearer entry should not be another form, got %q", conflict2)
	}
}

func writeJSONToFile(t *testing.T, path string, v any) {
	t.Helper()
	data := marshalForTest(t, v)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
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

func marshalForTest(t *testing.T, v any) []byte {
	t.Helper()
	data, err := marshalNoEscape(v)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
