//go:build devxe2e

package devx_test

import (
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

	n := time.Now().UnixNano()
	dsn := fmt.Sprintf("postgres://dxu%016x:dxp%016x@127.0.0.1:1/dxdb%016x?sslmode=disable",
		n, n^0x5a5a5a5a5a5a5a5a, n^0x2b2b2b2b2b2b2b2b)

	code25, out25 := runSAGWithInput(t, ctx, []byte(dsn),
		"credential", "create", "dxac25r",
		"--kind", "postgres",
		"--agent", "dxac25pi",
		"--tls", "disable",
		"--stdin",
		"--accept-failed-check",
	)
	if code25 != 0 {
		t.Fatalf("DXAC25: credential create dxac25r: exit %d; output: %s", code25, out25)
	}

	t.Cleanup(func() {
		if err := dxh.WaitLaunchAgentIdle(ctx, r, uid); err != nil {
			t.Logf("DXAC25: cleanup wait idle: %v", err)
			return
		}
		code, _ := deleteViaStartTTY(t, ctx, "dxac25r")
		if code != 0 {
			t.Logf("DXAC25: cleanup delete dxac25r: exit %d", code)
		}
	})

	if err := dxh.WaitLaunchAgentIdle(ctx, r, uid); err != nil {
		t.Fatalf("DXAC25: wait idle after create: %v", err)
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
		t.Fatal("DXAC25: jwt generate did not print a line")
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

	mcpBody := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"sql_list_tables","arguments":{"resource":"dxac25r"}}}`
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://localhost:8443/mcp",
		strings.NewReader(mcpBody))
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
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("DXAC25: read tools/call response: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("DXAC25: tools/call: HTTP %d; want 200", resp.StatusCode)
	}

	requestID := dxac25ExtractRequestID(t, respBody)
	if requestID == "" {
		t.Fatal("DXAC25: tools/call response has no request_id")
	}

	auditOut, err := setsidR.Output(ctx, bin, "logs", "audit")
	if err != nil {
		t.Fatalf("DXAC25: sag logs audit (setsid): %v", err)
	}

	count := 0
	for _, line := range strings.Split(string(auditOut), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var rec map[string]any
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			continue
		}
		if id, ok := rec["request_id"].(string); ok && id == requestID {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("DXAC25: audit has %d lines with request_id; want 1", count)
	}
}

func dxac25UID() string {
	return fmt.Sprintf("%d", os.Getuid())
}

func dxac25ExtractRequestID(t *testing.T, body []byte) string {
	t.Helper()
	bodyStr := string(body)
	var mcpJSON string
	for _, line := range strings.Split(bodyStr, "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.HasPrefix(line, "data: ") {
			mcpJSON = strings.TrimPrefix(line, "data: ")
			break
		}
	}
	if mcpJSON == "" {
		mcpJSON = strings.TrimSpace(bodyStr)
	}
	var rpcResp struct {
		Result struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(mcpJSON), &rpcResp); err != nil {
		t.Fatalf("DXAC25: parse tools/call response: %v", err)
	}
	if len(rpcResp.Result.Content) == 0 {
		return ""
	}
	var toolResult map[string]any
	if err := json.Unmarshal([]byte(rpcResp.Result.Content[0].Text), &toolResult); err != nil {
		t.Fatalf("DXAC25: parse tools/call content text: %v", err)
	}
	id, _ := toolResult["request_id"].(string)
	return id
}
