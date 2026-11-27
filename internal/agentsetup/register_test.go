package agentsetup

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/kanthorlabs/sag/internal/secret"
)

func makeTestJWT(sub string) string {
	claims := jwt.MapClaims{
		"sub":  sub,
		"name": sub,
		"kind": "agent",
		"iat":  time.Now().Unix(),
		"exp":  time.Now().Add(time.Hour).Unix(),
		"jti":  "testjti",
	}
	token, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(make([]byte, 32))
	return token
}

func TestNoCommandEntry(t *testing.T) {
	dir := t.TempDir()
	mcpFile := filepath.Join(dir, ".claude.json")
	entry := map[string]any{
		"mcpServers": map[string]any{
			"sag": map[string]any{
				"command": "sag",
				"url":     "https://localhost:8443/mcp",
			},
		},
	}
	data, _ := json.Marshal(entry)
	os.WriteFile(mcpFile, data, 0o600)

	opts := Options{
		Harness:    HarnessClaudeCode,
		Sub:        "claude-code",
		URL:        "https://localhost:8443/mcp",
		CfgDir:     dir,
		PodmanPath: "podman",
		Env:        map[string]string{"CLAUDE_CONFIG_DIR": dir},
		Home:       dir,
	}
	conflicts := planConflicts(opts, ModeLocal, true)
	found := false
	for _, c := range conflicts {
		if containsString(c, "another form") {
			found = true
		}
	}
	if !found {
		t.Errorf("conflicts = %v; want an 'another form' conflict for command entry", conflicts)
	}
}

func TestKeptEntryByteEqual(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	dir := t.TempDir()
	mcpFile := filepath.Join(dir, ".claude.json")
	keptJWT := makeTestJWT("claude-code")

	entry := map[string]any{
		"mcpServers": map[string]any{
			"sag": map[string]any{
				"type": "http",
				"url":  srv.URL,
				"headers": map[string]string{
					"Authorization": "Bearer " + keptJWT,
				},
			},
		},
	}
	data, _ := json.Marshal(entry)
	os.WriteFile(mcpFile, data, 0o600)
	before, _ := os.ReadFile(mcpFile)

	var out bytes.Buffer
	SetOut(&out)
	defer SetOut(os.Stdout)

	origProbe := mcpProbe
	mcpProbe = func(_ context.Context, url, _ string, cred secret.Credential) (int, error) {
		return http.StatusOK, nil
	}
	defer func() { mcpProbe = origProbe }()

	cfgDir := t.TempDir()
	pemData := saveTLSCertPEM(t, srv)
	os.WriteFile(filepath.Join(cfgDir, "ca.pem"), pemData, 0o600)

	opts := Options{
		Harness:    HarnessClaudeCode,
		Sub:        "claude-code",
		URL:        srv.URL,
		CfgDir:     cfgDir,
		PodmanPath: "podman",
		Env:        map[string]string{"CLAUDE_CONFIG_DIR": dir},
		Home:       dir,
	}
	newCred := secret.NewCredential("jwt-claude-code", []byte(makeTestJWT("other")))
	if err := registerMCP(context.Background(), opts, newCred); err != nil {
		t.Fatalf("registerMCP: %v", err)
	}

	after, _ := os.ReadFile(mcpFile)
	if !bytes.Equal(before, after) {
		t.Errorf("file changed when entry should be kept\nbefore: %s\nafter: %s", before, after)
	}
}

func TestOtherSubNewJWT(t *testing.T) {
	dir := t.TempDir()
	mcpFile := filepath.Join(dir, ".claude.json")
	keptJWT := makeTestJWT("other-sub")
	const url = "https://localhost:8443/mcp"

	entry := map[string]any{
		"mcpServers": map[string]any{
			"sag": map[string]any{
				"type": "http",
				"url":  url,
				"headers": map[string]string{
					"Authorization": "Bearer " + keptJWT,
				},
			},
		},
	}
	data, _ := json.Marshal(entry)
	os.WriteFile(mcpFile, data, 0o600)

	var out bytes.Buffer
	SetOut(&out)
	defer SetOut(os.Stdout)

	newJWT := makeTestJWT("claude-code")
	newCred := secret.NewCredential("jwt-claude-code", []byte(newJWT))

	cfgDir := t.TempDir()
	os.WriteFile(filepath.Join(cfgDir, "ca.pem"), []byte("fake"), 0o600)

	opts := Options{
		Harness:    HarnessClaudeCode,
		Sub:        "claude-code",
		URL:        url,
		CfgDir:     cfgDir,
		PodmanPath: "podman",
		Env:        map[string]string{"CLAUDE_CONFIG_DIR": dir},
		Home:       dir,
	}
	if err := registerMCP(context.Background(), opts, newCred); err != nil {
		t.Fatalf("registerMCP: %v", err)
	}

	after, _ := os.ReadFile(mcpFile)
	if bytes.Contains(after, []byte(keptJWT)) {
		t.Error("file still has old JWT after sub mismatch; want new entry")
	}
	if !bytes.Contains(after, []byte(newJWT)) {
		t.Error("file missing new JWT")
	}
}

func TestUnauthorizedNewJWT(t *testing.T) {
	dir := t.TempDir()
	mcpFile := filepath.Join(dir, ".claude.json")
	keptJWT := makeTestJWT("claude-code")
	const url = "https://localhost:8443/mcp"

	entry := map[string]any{
		"mcpServers": map[string]any{
			"sag": map[string]any{
				"type": "http",
				"url":  url,
				"headers": map[string]string{
					"Authorization": "Bearer " + keptJWT,
				},
			},
		},
	}
	data, _ := json.Marshal(entry)
	os.WriteFile(mcpFile, data, 0o600)

	var out bytes.Buffer
	SetOut(&out)
	defer SetOut(os.Stdout)

	origProbe := mcpProbe
	mcpProbe = func(_ context.Context, _, _ string, _ secret.Credential) (int, error) {
		return http.StatusUnauthorized, nil
	}
	defer func() { mcpProbe = origProbe }()

	newJWT := makeTestJWT("claude-code")
	newCred := secret.NewCredential("jwt-claude-code", []byte(newJWT))

	cfgDir := t.TempDir()
	os.WriteFile(filepath.Join(cfgDir, "ca.pem"), []byte("fake"), 0o600)

	opts := Options{
		Harness:    HarnessClaudeCode,
		Sub:        "claude-code",
		URL:        url,
		CfgDir:     cfgDir,
		PodmanPath: "podman",
		Env:        map[string]string{"CLAUDE_CONFIG_DIR": dir},
		Home:       dir,
	}
	if err := registerMCP(context.Background(), opts, newCred); err != nil {
		t.Fatalf("registerMCP: %v", err)
	}

	after, _ := os.ReadFile(mcpFile)
	if !bytes.Contains(after, []byte(newJWT)) {
		t.Error("file missing new JWT after 401 response")
	}
}

func TestJWTNotInArgsOrEnv(t *testing.T) {
	var capturedArgs []string
	origExec := execPodmanExec
	execPodmanExec = func(_ context.Context, path string, args []string) ([]byte, error) {
		capturedArgs = args
		return []byte(makeTestJWT("claude-code") + "\n"), nil
	}
	defer func() { execPodmanExec = origExec }()

	ctx := context.Background()
	opts := Options{
		Harness:    HarnessClaudeCode,
		Sub:        "claude-code",
		URL:        "https://localhost:8443/mcp",
		CfgDir:     t.TempDir(),
		PodmanPath: "podman",
		Env:        map[string]string{"CLAUDE_CONFIG_DIR": t.TempDir()},
		Home:       t.TempDir(),
	}
	cred, err := issueJWT(ctx, opts)
	if err != nil {
		t.Fatalf("issueJWT: %v", err)
	}
	jwtStr := string(cred.Reveal())
	for _, a := range capturedArgs {
		if containsString(a, jwtStr) {
			t.Errorf("JWT found in arg %q", a)
		}
	}
	for _, e := range os.Environ() {
		if containsString(e, jwtStr) {
			t.Errorf("JWT found in env %q", e)
		}
	}
}

func saveTLSCertPEM(t *testing.T, srv *httptest.Server) []byte {
	t.Helper()
	var out []byte
	for _, cert := range srv.TLS.Certificates {
		for _, der := range cert.Certificate {
			out = append(out, pem.EncodeToMemory(&pem.Block{
				Type:  "CERTIFICATE",
				Bytes: der,
			})...)
		}
	}
	return out
}
