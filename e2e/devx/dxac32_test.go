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
	awsTests["DXAC32_AWSUnavailable"] = func(t *testing.T, ctx context.Context) {
		DXAC32_AWSUnavailable(t, ctx)
	}
}

const (
	ac32LocalAgent = "dxac32-agent"
	ac32LocalRes   = "dxac32-local"
	ac32LocalCred  = "sag-res-dxac32-local"

	ac32AWSRegion    = "us-east-1"
	ac32AWSEndpoint  = "http://localhost:4566"
	ac32AWSCred      = "sag-aws-credentials"
	ac32LSContainer  = "localstack-main"

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

	dsn32 := "postgres://ac32:ac32pw-7q3x@127.0.0.1:5432/ac32"
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

	awsArgs := []string{
		"--endpoint-url", ac32AWSEndpoint,
		"--region", ac32AWSRegion,
	}
	for _, name := range []string{ac32Secret1, ac32Secret2, ac32Secret3} {
		createArgs := append([]string{"secretsmanager", "create-secret",
			"--name", name,
			"--secret-string", "postgres://awsuser:awspw-9b4z@aws-db.internal:5432/awsdb"}, awsArgs...)
		if _, err := r.Output(ctx, "aws", createArgs...); err != nil {
			t.Fatalf("DXAC32: aws create-secret %s: %v", name, err)
		}
		t.Cleanup(func() {
			cctx := context.Background()
			deleteArgs := append([]string{"secretsmanager", "delete-secret",
				"--secret-id", name, "--force-delete-without-recovery"}, awsArgs...)
			_, _ = r.Output(cctx, "aws", deleteArgs...)
		})
	}

	sagYAML, err := os.ReadFile(sagYAMLPath)
	if err != nil {
		t.Fatalf("DXAC32: read sag.yaml: %v", err)
	}
	resourceBlock := fmt.Sprintf(`storage:
  aws:
    region: %s
    credentials: secret:%s
resources:
  %s:
    kind: postgres
    connection: secret:%s
    agents: [%s]
  %s:
    kind: postgres
    connection: aws-sm:%s
    agents: [%s]
  %s:
    kind: postgres
    connection: aws-sm:%s
    agents: [%s]
  %s:
    kind: postgres
    connection: aws-sm:%s
    agents: [%s]`, ac32AWSRegion, ac32AWSCred,
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

	for _, res := range []string{ac32AWSRes1, ac32AWSRes2, ac32AWSRes3} {
		if !ac32ResourceUnavailable(ctx, t, mcpClient, jwt, res) {
			t.Errorf("DXAC32: resource %s should be unavailable while LocalStack paused", res)
		}
	}

	if ac32ResourceUnavailable(ctx, t, mcpClient, jwt, ac32LocalRes) {
		t.Errorf("DXAC32: local Podman resource %s should be served while LocalStack paused", ac32LocalRes)
	}

	if _, err := r.Output(ctx, "podman", "unpause", ac32LSContainer); err != nil {
		t.Fatalf("DXAC32: podman unpause %s: %v", ac32LSContainer, err)
	}

	deadline := time.Now().Add(120 * time.Second)
	for _, res := range []string{ac32AWSRes1, ac32AWSRes2, ac32AWSRes3} {
		for {
			if !ac32ResourceUnavailable(ctx, t, mcpClient, jwt, res) {
				break
			}
			if time.Now().After(deadline) {
				t.Errorf("DXAC32: resource %s still unavailable 120s after LocalStack unpause", res)
				break
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(5 * time.Second):
			}
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

func ac32ResourceUnavailable(ctx context.Context, t *testing.T, client *http.Client, jwt, resource string) bool {
	t.Helper()
	body, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "tools/call",
		"params": map[string]any{
			"name":      "sql_list_tables",
			"arguments": map[string]any{"resource": resource},
		},
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://localhost:8443/mcp",
		strings.NewReader(string(body)))
	if err != nil {
		return true
	}
	req.Header.Set("Authorization", "Bearer "+jwt)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Mcp-Protocol-Version", "2025-06-18")
	resp, err := client.Do(req)
	if err != nil {
		return true
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)
	return strings.Contains(string(out), "resource_unavailable")
}
