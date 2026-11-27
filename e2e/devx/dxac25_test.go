//go:build devxe2e

package devx_test

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kanthorlabs/sag/e2e/devx/dxh"
)

func init() {
	hostformsTests["DXAC25_Logs"] = func(t *testing.T, ctx context.Context) {
		DXAC25_Logs(t, ctx)
	}
}

func DXAC25_Logs(t *testing.T, ctx context.Context) {
	t.Helper()
	dxh.EnsureBootstrapped(t)

	home := os.Getenv("HOME")
	cfgDir := dxh.CfgDir(home)
	uid := dxac25UID()

	r := dxh.NewRunner()
	if err := dxh.WaitLaunchAgentIdle(ctx, r, uid); err != nil {
		t.Fatalf("DXAC25: wait launch agent idle: %v", err)
	}

	bin, err := dxh.TestBinary()
	if err != nil {
		t.Fatalf("DXAC25: test binary: %v", err)
	}
	groupDir := t.TempDir()
	binDir := t.TempDir()

	setsidR := dxh.NewSetsidRunner(groupDir, binDir)

	daemonOut, err := setsidR.Output(ctx, "sh", "-c", `exec "$0" "$@" 2>&1`, bin, "logs", "daemon", "--tail", "200")
	if err != nil {
		t.Fatalf("DXAC25: sag logs daemon (setsid): %v", err)
	}
	if !strings.Contains(string(daemonOut), "sag starting") {
		t.Fatalf("DXAC25: sag logs daemon output has no 'sag starting'")
	}

	dxac25Secret := "sag-res-dxac25r-conn"
	secretFile := filepath.Join(t.TempDir(), "dxac25-conn")
	if err := os.WriteFile(secretFile, []byte("postgres://dxac25test:dxac25pass@127.0.0.1:65432/dxac25testdb?sslmode=disable"), 0o600); err != nil {
		t.Fatalf("DXAC25: write secret file: %v", err)
	}
	if _, err := r.Output(ctx, "podman", "secret", "create",
		"--label", "sag.devx-e2e=1",
		"--label", "sag.resource=dxac25r",
		dxac25Secret, secretFile); err != nil {
		t.Fatalf("DXAC25: create secret: %v", err)
	}

	sagYAMLPath := filepath.Join(cfgDir, "sag.yaml")
	sagYAML, err := os.ReadFile(sagYAMLPath)
	if err != nil {
		t.Fatalf("DXAC25: read sag.yaml: %v", err)
	}
	resourceBlock := "  dxac25r:\n    kind: postgres\n    connection: secret:" + dxac25Secret + "\n    agents:\n    - dxac25pi"
	yamlContent := string(sagYAML)
	var modified string
	switch {
	case strings.Contains(yamlContent, "dxac25r:"):
		modified = yamlContent
	case strings.Contains(yamlContent, "resources: {}"):
		modified = strings.Replace(yamlContent, "resources: {}", "resources:\n"+resourceBlock, 1)
	default:
		idx := strings.Index(yamlContent, "\nresources:\n")
		if idx >= 0 {
			insertAt := idx + len("\nresources:\n")
			modified = yamlContent[:insertAt] + resourceBlock + "\n" + yamlContent[insertAt:]
		} else {
			modified = strings.TrimRight(yamlContent, "\n") + "\nresources:\n" + resourceBlock + "\n"
		}
	}
	if err := os.WriteFile(sagYAMLPath, []byte(modified), 0o644); err != nil {
		t.Fatalf("DXAC25: write sag.yaml: %v", err)
	}

	if err := dxh.WaitLaunchAgentIdle(ctx, r, uid); err != nil {
		t.Fatalf("DXAC25: wait idle before restart: %v", err)
	}
	if out, err := runSAG(t, ctx, "service", "stop"); err != nil {
		t.Fatalf("DXAC25: service stop: %v\n%s", err, out)
	}
	if out, err := runSAG(t, ctx, "service", "start"); err != nil {
		t.Fatalf("DXAC25: service start: %v\n%s", err, out)
	}

	jwtGroupDir := t.TempDir()
	jwtBinDir := t.TempDir()
	tty, err := dxh.StartTTY(ctx, jwtGroupDir, jwtBinDir, "podman", "exec", "-it", "sag", "sag", "jwt", "generate", "dxac25pi")
	if err != nil {
		t.Fatalf("DXAC25: start jwt generate tty: %v", err)
	}
	if err := tty.Expect(ctx, "\n"); err != nil {
		t.Fatalf("DXAC25: jwt generate did not print a line: %v", err)
	}
	raw := tty.RawOutput()
	jwt := jwtRe.FindString(raw)
	if jwt != "" {
		tty.Redact(jwt)
	}
	jwtCode, waitErr := tty.Wait(ctx)
	if waitErr != nil {
		t.Fatalf("DXAC25: jwt generate wait: %v", waitErr)
	}
	if jwtCode != 0 {
		t.Fatalf("DXAC25: jwt generate exit %d; want 0", jwtCode)
	}
	if jwt == "" {
		t.Fatal("DXAC25: jwt generate: no JWT found in terminal output")
	}

	jti := dxac25JTIFromJWT(t, jwt)

	caPath := filepath.Join(cfgDir, "ca.pem")
	caPEM, err := os.ReadFile(caPath)
	if err != nil {
		t.Fatalf("DXAC25: read ca.pem: %v", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		t.Fatal("DXAC25: ca.pem: no certificate found")
	}
	mcpClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{RootCAs: pool},
		},
		Timeout: 15 * time.Second,
	}

	body := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"list_resources","arguments":{}}}`
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://localhost:8443/mcp",
		strings.NewReader(body))
	if err != nil {
		t.Fatalf("DXAC25: build MCP request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+jwt)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Mcp-Protocol-Version", "2025-06-18")

	resp, err := mcpClient.Do(req)
	if err != nil {
		t.Fatalf("DXAC25: tools/call: %v", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("DXAC25: tools/call: HTTP %d; want 200", resp.StatusCode)
	}

	auditOut, err := setsidR.Output(ctx, bin, "logs", "audit")
	if err != nil {
		t.Fatalf("DXAC25: sag logs audit (setsid): %v", err)
	}

	requestID := dxac25FindRequestID(string(auditOut), jti)
	if requestID == "" {
		t.Fatalf("DXAC25: audit output has no line with jti matching the tools/call")
	}
}

func dxac25UID() string {
	return fmt.Sprintf("%d", os.Getuid())
}

func dxac25JTIFromJWT(t *testing.T, raw string) string {
	t.Helper()
	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		t.Fatalf("DXAC25: JWT does not have 3 parts")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatalf("DXAC25: decode JWT payload: %v", err)
	}
	var claims map[string]any
	if err := json.Unmarshal(payload, &claims); err != nil {
		t.Fatalf("DXAC25: unmarshal JWT claims: %v", err)
	}
	jti, _ := claims["jti"].(string)
	if jti == "" {
		t.Fatal("DXAC25: JWT has no jti claim")
	}
	return jti
}

func dxac25FindRequestID(auditOutput, jti string) string {
	for _, line := range strings.Split(auditOutput, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var rec map[string]any
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			continue
		}
		if rec["jti"] == jti {
			if id, ok := rec["request_id"].(string); ok && id != "" {
				return id
			}
		}
	}
	return ""
}
