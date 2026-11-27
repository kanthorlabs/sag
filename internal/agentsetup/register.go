package agentsetup

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/kanthorlabs/sag/internal/secret"
	"github.com/kanthorlabs/sag/internal/stepline"
)

const initializeBody = `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"sag-agent-setup","version":"1"}}}`

var mcpProbe = func(ctx context.Context, url, caPath string, cred secret.Credential) (int, error) {
	caPEM, err := os.ReadFile(caPath)
	if err != nil {
		return 0, fmt.Errorf("read ca.pem: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		return 0, fmt.Errorf("ca.pem: no certificate")
	}
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{RootCAs: pool},
		},
		Timeout: 10 * time.Second,
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url,
		bytes.NewBufferString(initializeBody))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+string(cred.Reveal()))
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	return resp.StatusCode, nil
}

type mcpServerEntry struct {
	Type     string            `json:"type,omitempty"`
	URL      string            `json:"url"`
	Headers  map[string]string `json:"headers"`
	Exposure string            `json:"exposure,omitempty"`
}

func registerMCP(ctx context.Context, opts Options, jwtCred secret.Credential) error {
	paths := harnessPathsFromOpts(opts)
	out := stepline.New(agentOut)

	mcpFile := paths.MCPFile
	mcpDir := filepath.Dir(mcpFile)
	if err := os.MkdirAll(mcpDir, 0o700); err != nil {
		return err
	}

	caPath := filepath.Join(opts.CfgDir, "ca.pem")

	keptJWT, keptURL := readKeptEntry(mcpFile, opts.Harness)
	if keptJWT != "" && keptURL == opts.URL {
		keptSub := subFromRawJWT(keptJWT)
		if keptSub == opts.Sub {
			keptCred := secret.NewCredential("jwt-"+opts.Sub, []byte(keptJWT))
			status, err := mcpProbe(ctx, opts.URL, caPath, keptCred)
			if err == nil && status != http.StatusUnauthorized {
				out.Line(stepline.StatusOK, "mcp-server", "")
				return nil
			}
		}
	}

	entry := buildEntry(opts.Harness, opts.URL, jwtCred)
	var keyPath []string
	switch opts.Harness {
	case HarnessPi:
		keyPath = []string{"mcpServers", "sag"}
	default:
		keyPath = []string{"mcpServers", "sag"}
	}
	if err := writeJSON(mcpFile, keyPath, entry); err != nil {
		return err
	}
	if err := ensureMode0600(mcpFile); err != nil {
		return err
	}
	out.Line(stepline.StatusOK, "mcp-server", "")
	return nil
}

func buildEntry(h Harness, url string, cred secret.Credential) any {
	bearer := "Bearer " + string(cred.Reveal())
	switch h {
	case HarnessPi:
		return map[string]any{
			"url":      url,
			"headers":  map[string]string{"Authorization": bearer},
			"exposure": "direct",
		}
	default:
		return map[string]any{
			"type":    "http",
			"url":     url,
			"headers": map[string]string{"Authorization": bearer},
		}
	}
}

func readKeptEntry(mcpFile string, h Harness) (jwtStr, url string) {
	data, err := os.ReadFile(mcpFile)
	if err != nil {
		return "", ""
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(data, &top); err != nil {
		return "", ""
	}
	mcpRaw, ok := top["mcpServers"]
	if !ok {
		return "", ""
	}
	var servers map[string]json.RawMessage
	if err := json.Unmarshal(mcpRaw, &servers); err != nil {
		return "", ""
	}
	sagRaw, ok := servers["sag"]
	if !ok {
		return "", ""
	}
	var entry map[string]json.RawMessage
	if err := json.Unmarshal(sagRaw, &entry); err != nil {
		return "", ""
	}
	if _, hasCmd := entry["command"]; hasCmd {
		return "", ""
	}
	urlRaw, ok := entry["url"]
	if !ok {
		return "", ""
	}
	var entryURL string
	if err := json.Unmarshal(urlRaw, &entryURL); err != nil {
		return "", ""
	}
	headersRaw, ok := entry["headers"]
	if !ok {
		return "", ""
	}
	var headers map[string]string
	if err := json.Unmarshal(headersRaw, &headers); err != nil {
		return "", ""
	}
	auth := headers["Authorization"]
	if !strings.HasPrefix(auth, "Bearer ") {
		return "", ""
	}
	token := strings.TrimPrefix(auth, "Bearer ")
	return token, entryURL
}

func subFromRawJWT(raw string) string {
	p := jwt.NewParser(jwt.WithoutClaimsValidation(), jwt.WithJSONNumber())
	var claims jwt.MapClaims
	_, _, err := p.ParseUnverified(raw, &claims)
	if err != nil {
		return ""
	}
	sub, _ := claims["sub"].(string)
	return sub
}

func ensureMode0600(path string) error {
	fi, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if fi.Mode()&0o777 != 0o600 {
		return os.Chmod(path, 0o600)
	}
	return nil
}

func harnessPathsFromOpts(opts Options) HarnessPaths {
	switch opts.Harness {
	case HarnessPi:
		return PiPaths(opts.Env["PI_CODING_AGENT_DIR"], opts.Home)
	default:
		return ClaudeCodePaths(opts.Env["CLAUDE_CONFIG_DIR"], opts.Home)
	}
}
