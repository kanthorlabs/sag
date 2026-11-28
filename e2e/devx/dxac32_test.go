//go:build devxe2e

package devx_test

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
	"testing"
	"time"

	"github.com/kanthorlabs/sag/e2e/devx/dxh"
)

func init() {
	awsTests["DXAC32_AWSUnavailable"] = func(t *testing.T, ctx context.Context) {
		DXAC32_AWSUnavailable(t, ctx)
	}
}

const (
	ac32LocalAgent = "dxac32-agent"
	ac32LocalRes   = "dxac32-local"
	ac32LocalCred  = "sag-res-dxac32-local"

	ac32AWSCred     = "sag-aws-credentials"
	ac32LSContainer = "sag-localstack"

	ac32AWSRes1 = "dxac32-aws-db1"
	ac32AWSRes2 = "dxac32-aws-db2"
	ac32AWSRes3 = "dxac32-aws-db3"

	ac32Secret1 = "dxac32/db1/conn"
	ac32Secret2 = "dxac32/db2/conn"
	ac32Secret3 = "dxac32/db3/conn"
)

func DXAC32_AWSUnavailable(t *testing.T, ctx context.Context) {
	t.Helper()
	dxh.EnsureBootstrapped(t)
	dxh.EnsureLocalStack(t)

	r := dxh.NewRunner()
	home := os.Getenv("HOME")
	uid := fmt.Sprintf("%d", os.Getuid())
	cfgDir := dxh.CfgDir(home)
	sagYAMLPath := filepath.Join(cfgDir, "sag.yaml")

	if err := dxh.WaitLaunchAgentIdle(ctx, r, uid); err != nil {
		t.Fatalf("DXAC32: wait launch agent idle: %v", err)
	}

	pgDSN, pgErr := dxh.StartAWSTestPostgres(ctx, r, "sag-dxac32-pg")
	if pgErr != nil {
		t.Fatalf("DXAC32: start test postgres: %v", pgErr)
	}
	t.Cleanup(func() {
		cctx := context.Background()
		_, _ = r.Output(cctx, "podman", "stop", "sag-dxac32-pg")
	})

	dsn32 := pgDSN
	connFile := filepath.Join(t.TempDir(), "dxac32-local-conn")
	if err := os.WriteFile(connFile, []byte(dsn32), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Output(ctx, "podman", "secret", "create",
		"--label", "sag.devx-e2e=1",
		"--label", "sag.resource=dxac32-local",
		ac32LocalCred, connFile); err != nil {
		t.Fatalf("DXAC32: podman secret create %s: %v", ac32LocalCred, err)
	}
	t.Cleanup(func() {
		cctx := context.Background()
		_, _ = r.Output(cctx, "podman", "secret", "rm", ac32LocalCred)
	})

	for _, name := range []string{ac32Secret1, ac32Secret2, ac32Secret3} {
		if err := dxh.LocalStackCreateSecret(ctx, name, pgDSN); err != nil {
			t.Fatalf("DXAC32: create AWS secret %s: %v", name, err)
		}
		t.Cleanup(func() {
			if err := dxh.LocalStackDeleteSecret(context.Background(), name); err != nil {
				t.Errorf("DXAC32: delete AWS secret %s: %v", name, err)
			}
		})
	}

	sagYAML, err := os.ReadFile(sagYAMLPath)
	if err != nil {
		t.Fatalf("DXAC32: read sag.yaml: %v", err)
	}
	resourceBlock := fmt.Sprintf(`resources:
  %s:
    kind: postgres
    connection: secret:%s
    tls: disable
    agents: [%s]
  %s:
    kind: postgres
    connection: aws-sm:%s
    tls: disable
    agents: [%s]
  %s:
    kind: postgres
    connection: aws-sm:%s
    tls: disable
    agents: [%s]
  %s:
    kind: postgres
    connection: aws-sm:%s
    tls: disable
    agents: [%s]`,
		ac32LocalRes, ac32LocalCred, ac32LocalAgent,
		ac32AWSRes1, ac32Secret1, ac32LocalAgent,
		ac32AWSRes2, ac32Secret2, ac32LocalAgent,
		ac32AWSRes3, ac32Secret3, ac32LocalAgent)
	modified := strings.Replace(string(sagYAML), "resources: {}", resourceBlock, 1)
	if modified == string(sagYAML) {
		t.Fatal("DXAC32: sag.yaml does not contain 'resources: {}'; cannot add ac32 resources")
	}
	if err := os.WriteFile(sagYAMLPath, []byte(modified), 0o644); err != nil {
		t.Fatalf("DXAC32: write sag.yaml: %v", err)
	}
	t.Cleanup(func() {
		if err := os.WriteFile(sagYAMLPath, sagYAML, 0o644); err != nil {
			t.Errorf("DXAC32: restore sag.yaml: %v", err)
		}
	})

	if _, err := runSAG(t, ctx, "service", "stop"); err != nil {
		t.Fatalf("DXAC32: service stop before pause: %v", err)
	}

	if _, err := r.Output(ctx, "podman", "pause", ac32LSContainer); err != nil {
		t.Fatalf("DXAC32: podman pause %s: %v", ac32LSContainer, err)
	}
	t.Cleanup(func() {
		cctx := context.Background()
		_, _ = r.Output(cctx, "podman", "unpause", ac32LSContainer)
	})

	if _, err := runSAG(t, ctx, "service", "start"); err != nil {
		t.Fatalf("DXAC32: service start with LocalStack paused: %v", err)
	}

	caPath := filepath.Join(cfgDir, "ca.pem")
	if err := healthzOK(ctx, caPath); err != nil {
		t.Fatalf("DXAC32: healthz with LocalStack paused: %v", err)
	}

	jwt := ac32GetJWT(t, ctx, ac32LocalAgent)
	caPool := ac32CAPool(t, caPath)
	mcpClient := &http.Client{
		Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: caPool}},
		Timeout:   15 * time.Second,
	}

	ac32WaitServed(t, ctx, mcpClient, jwt, ac32LocalRes, time.Now().Add(60*time.Second), "after service start")

	for _, res := range []string{ac32AWSRes1, ac32AWSRes2, ac32AWSRes3} {
		if state := ac32ResourceState(ctx, mcpClient, jwt, res); state != ac32Unavailable {
			t.Errorf("DXAC32: resource %s while LocalStack paused: %s; want %s", res, state, ac32Unavailable)
		}
	}

	if _, err := r.Output(ctx, "podman", "unpause", ac32LSContainer); err != nil {
		t.Fatalf("DXAC32: podman unpause %s: %v", ac32LSContainer, err)
	}

	deadline := time.Now().Add(120 * time.Second)
	for _, res := range []string{ac32AWSRes1, ac32AWSRes2, ac32AWSRes3} {
		ac32WaitServed(t, ctx, mcpClient, jwt, res, deadline, "within 120s after LocalStack unpause")
	}
}

const (
	ac32Served      = "served"
	ac32Unavailable = "resource_unavailable"
)

func ac32WaitServed(t *testing.T, ctx context.Context, client *http.Client, jwt, res string, deadline time.Time, when string) {
	t.Helper()
	for {
		state := ac32ResourceState(ctx, client, jwt, res)
		if state == ac32Served {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("DXAC32: resource %s not served %s; last state: %s", res, when, state)
		}
		select {
		case <-ctx.Done():
			t.Fatalf("DXAC32: resource %s: %v", res, ctx.Err())
		case <-time.After(2 * time.Second):
		}
	}
}

func ac32GetJWT(t *testing.T, ctx context.Context, agent string) string {
	t.Helper()
	gd := t.TempDir()
	bd := t.TempDir()
	tty, err := dxh.StartTTY(ctx, gd, bd, "podman", "exec", "-it", "sag", "sag", "jwt", "generate", agent)
	if err != nil {
		t.Fatalf("DXAC32: start jwt tty: %v", err)
	}
	if err := tty.Expect(ctx, "\n"); err != nil {
		t.Fatalf("DXAC32: jwt generate output: %v", err)
	}
	jwt := jwtRe.FindString(tty.RawOutput())
	if jwt != "" {
		tty.Redact(jwt)
	}
	code, err := tty.Wait(ctx)
	if err != nil {
		t.Fatalf("DXAC32: jwt wait: %v", err)
	}
	if code != 0 {
		t.Fatalf("DXAC32: jwt generate exit %d", code)
	}
	if jwt == "" {
		t.Fatal("DXAC32: no JWT in jwt generate output")
	}
	return jwt
}

func ac32CAPool(t *testing.T, caPath string) *x509.CertPool {
	t.Helper()
	pem, err := os.ReadFile(caPath)
	if err != nil {
		t.Fatalf("DXAC32: read ca.pem: %v", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pem) {
		t.Fatal("DXAC32: ca.pem: no certificate")
	}
	return pool
}

func ac32ResourceState(ctx context.Context, client *http.Client, jwt, resource string) string {
	body, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "tools/call",
		"params": map[string]any{
			"name":      "sql_list_tables",
			"arguments": map[string]any{"resource": resource},
		},
	})
	if err != nil {
		return "request: " + err.Error()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://localhost:8443/mcp", bytes.NewReader(body))
	if err != nil {
		return "request: " + err.Error()
	}
	req.Header.Set("Authorization", "Bearer "+jwt)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Mcp-Protocol-Version", "2025-06-18")
	resp, err := client.Do(req)
	if err != nil {
		return "transport: " + err.Error()
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "read: " + err.Error()
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Sprintf("http status %d", resp.StatusCode)
	}
	return ac32ParseState(raw)
}

func ac32ParseState(raw []byte) string {
	payload := raw
	for _, line := range bytes.Split(raw, []byte("\n")) {
		if data, ok := bytes.CutPrefix(bytes.TrimSpace(line), []byte("data:")); ok {
			payload = bytes.TrimSpace(data)
		}
	}
	var msg struct {
		Result *struct {
			IsError bool `json:"isError"`
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"result"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(payload, &msg); err != nil {
		return "unparsed response"
	}
	if msg.Error != nil {
		return "rpc error: " + msg.Error.Message
	}
	if msg.Result == nil {
		return "no result"
	}
	if !msg.Result.IsError {
		return ac32Served
	}
	if len(msg.Result.Content) != 1 {
		return "tool error without one content item"
	}
	var te struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal([]byte(msg.Result.Content[0].Text), &te); err != nil {
		return "unparsed tool error"
	}
	if te.Code == ac32Unavailable {
		return ac32Unavailable
	}
	return "tool error " + te.Code
}
