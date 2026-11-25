package daemon

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kanthorlabs/sag/internal/audit"
	"github.com/kanthorlabs/sag/internal/core"
	"github.com/kanthorlabs/sag/internal/keys"
	"github.com/kanthorlabs/sag/internal/secret"
	"github.com/kanthorlabs/sag/internal/sql"
	"github.com/kanthorlabs/sag/internal/testenv"
	"github.com/kanthorlabs/sag/internal/token"
)

const (
	leakKind     = "daemon-leak"
	leakSub      = "leaker"
	leakBroken   = "broken"
	leakServed   = "served"
	leakRedacted = "[REDACTED]"
)

type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

type leakResource struct {
	dsn      string
	executed *atomic.Int32
}

func (r *leakResource) Preflight(context.Context) (core.PreflightReport, error) {
	return core.PreflightReport{Passed: true}, nil
}

func (r *leakResource) Execute(_ context.Context, _, _ string, input json.RawMessage, _ core.Limits) (core.Result, error) {
	r.executed.Add(1)
	var in struct {
		SQL string `json:"sql"`
	}
	_ = json.Unmarshal(input, &in)
	return core.Result{}, &core.Error{
		Code:    core.CodeResourceUnavailable,
		Message: "resource unavailable",
		Trace:   core.Trace{Input: in.SQL},
		Cause:   errors.New("dial " + r.dsn + ": connection refused"),
	}
}

func (r *leakResource) Close() error { return nil }

type leakAdapter struct{ executed atomic.Int32 }

func (a *leakAdapter) Kind() string { return leakKind }

func (a *leakAdapter) Capabilities() []core.Capability { return []core.Capability{sql.Capability()} }

func (a *leakAdapter) Open(_ context.Context, cfg core.ResourceConfig, cred secret.Credential) (core.Resource, error) {
	if cfg.Name == leakBroken {
		return nil, fmt.Errorf("connect %s: password authentication failed", cred.Reveal())
	}
	return &leakResource{dsn: string(cred.Reveal()), executed: &a.executed}, nil
}

var registeredLeak = &leakAdapter{}

func init() {
	core.Register(registeredLeak)
}

func freeAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	if err := ln.Close(); err != nil {
		t.Fatal(err)
	}
	return addr
}

func replaceSecret(t *testing.T, dir, name string, value []byte) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, value, 0o400); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o400); err != nil {
		t.Fatal(err)
	}
}

func TestRunScrubsLogAndAudit(t *testing.T) {
	master := bytes.Repeat([]byte{9}, 32)
	f := newFixture(t, master)
	addr := freeAddr(t)
	cfg := fmt.Sprintf(`version: 1
server:
  listen: %s
  tls_cert: secret:sag-tls-cert
  tls_key: secret:sag-tls-key
resources:
  %s:
    kind: %s
    connection: secret:sag-main
    agents: [%s]
  %s:
    kind: %s
    connection: secret:sag-main
    agents: [%s]
`, addr, leakBroken, leakKind, leakSub, leakServed, leakKind, leakSub)
	if err := os.WriteFile(f.opts.ConfigPath, []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	ca, err := testenv.NewCA()
	if err != nil {
		t.Fatal(err)
	}
	certPEM, keyPEM, err := ca.Issue("127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	replaceSecret(t, f.opts.SecretsDir, "sag-tls-cert", certPEM)
	replaceSecret(t, f.opts.SecretsDir, "sag-tls-key", keyPEM)
	logs := &syncBuffer{}
	f.opts.Log = logs

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- Run(ctx, f.opts) }()
	stop := sync.OnceValue(func() error {
		cancel()
		return <-done
	})
	t.Cleanup(func() { _ = stop() })

	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(ca.CertPEM)
	client := &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool}}}
	jwtKey, err := keys.JWTKey(master, 1)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := token.Issue(jwtKey, leakSub, "Leaker", time.Now(), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	statement := "SELECT '" + testDSNPassword + "'"
	call := func() string {
		body, err := json.Marshal(map[string]any{
			"jsonrpc": "2.0", "id": 1, "method": "tools/call",
			"params": map[string]any{"name": "sql_query", "arguments": map[string]any{"resource": leakServed, "sql": statement}},
		})
		if err != nil {
			t.Fatal(err)
		}
		req, err := http.NewRequest(http.MethodPost, "https://"+addr+"/mcp", bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer "+raw)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		resp, err := client.Do(req)
		if err != nil {
			return ""
		}
		defer resp.Body.Close()
		out, _ := io.ReadAll(resp.Body)
		return string(out)
	}

	var responses []string
	deadline := time.Now().Add(15 * time.Second)
	for registeredLeak.executed.Load() == 0 {
		if time.Now().After(deadline) {
			t.Fatalf("resource %s did not execute a request; log:\n%s", leakServed, logs.String())
		}
		responses = append(responses, call())
		time.Sleep(50 * time.Millisecond)
	}
	if err := stop(); err != nil {
		t.Fatalf("Run: %v", err)
	}

	auditLines, err := os.ReadFile(filepath.Join(f.opts.StateDir, audit.DirName, "audit-"+time.Now().UTC().Format("2006-01-02")+".jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	out := logs.String()
	for _, want := range []string{`"msg":"resource open failed"`, `"msg":"request resource unavailable"`, leakRedacted} {
		if !strings.Contains(out, want) {
			t.Fatalf("log does not hold %q:\n%s", want, out)
		}
	}
	if !strings.Contains(string(auditLines), `"sql":"SELECT '`+leakRedacted+`'"`) {
		t.Fatalf("audit does not hold the scrubbed statement:\n%s", auditLines)
	}
	for name, text := range map[string]string{"log": out, "response": strings.Join(responses, "\n"), "audit": string(auditLines)} {
		if strings.Contains(text, testDSNPassword) {
			t.Fatalf("%s holds the DSN password", name)
		}
	}
}
