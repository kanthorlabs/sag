//go:build devxe2e

package devx_test

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/kanthorlabs/sag/e2e/devx/dxh"
)

var jwtRe = regexp.MustCompile(`eyJ[A-Za-z0-9+/=_-]+\.[A-Za-z0-9+/=_-]+\.[A-Za-z0-9+/=_-]+`)

func DXAC03_BootstrapIdempotent(t *testing.T, ctx context.Context) {
	t.Helper()
	dxh.EnsureBootstrapped(t)

	home := os.Getenv("HOME")
	cfgDir := dxh.CfgDir(home)
	r := dxh.NewRunner()

	sagYAMLPath := filepath.Join(cfgDir, "sag.yaml")

	connFile := filepath.Join(t.TempDir(), "dx03-conn")
	if err := os.WriteFile(connFile, []byte("postgres://u:p@127.0.0.1:5432/db?sslmode=disable"), 0o600); err != nil {
		t.Fatalf("write dx03 conn file: %v", err)
	}
	if _, err := r.Output(ctx, "podman", "secret", "create",
		"--label", "sag.devx-e2e=1",
		"--label", "sag.resource=dx03",
		"sag-res-dx03-conn", connFile); err != nil {
		t.Fatalf("podman secret create sag-res-dx03-conn: %v", err)
	}

	sagYAML, err := os.ReadFile(sagYAMLPath)
	if err != nil {
		t.Fatalf("read sag.yaml: %v", err)
	}
	resourceBlock := `resources:
  dx03:
    kind: postgres
    connection: secret:sag-res-dx03-conn
    agents:
    - dx03-agent`
	modified := strings.Replace(string(sagYAML), "resources: {}", resourceBlock, 1)
	if modified == string(sagYAML) {
		t.Fatal("sag.yaml does not contain 'resources: {}'; cannot add dx03 resource")
	}
	if err := os.WriteFile(sagYAMLPath, []byte(modified), 0o644); err != nil {
		t.Fatalf("write sag.yaml: %v", err)
	}

	if err := waitLaunchAgentIdle(ctx, strconv.Itoa(os.Getuid())); err != nil {
		t.Fatalf("wait launch agent idle: %v", err)
	}
	if out, err := runSAG(t, ctx, "service", "stop"); err != nil {
		t.Fatalf("service stop: %v\n%s", err, out)
	}
	if out, err := runSAG(t, ctx, "service", "start"); err != nil {
		t.Fatalf("service start after resource add: %v\n%s", err, out)
	}

	groupDir := t.TempDir()
	binDir := t.TempDir()
	tty, err := dxh.StartTTY(ctx, groupDir, binDir, "podman", "exec", "-it", "sag", "sag", "jwt", "generate", "dx03-agent")
	if err != nil {
		t.Fatalf("start jwt generate tty: %v", err)
	}
	if err := tty.Expect(ctx, "\n"); err != nil {
		t.Fatalf("jwt generate did not print a line: %v", err)
	}
	raw := tty.RawOutput()
	jwt := jwtRe.FindString(raw)
	if jwt != "" {
		tty.Redact(jwt)
	}
	jwtCode, err := tty.Wait(ctx)
	if err != nil {
		t.Fatalf("jwt generate wait: %v", err)
	}
	if jwtCode != 0 {
		t.Fatalf("jwt generate exit %d; want 0", jwtCode)
	}
	if jwt == "" {
		t.Fatal("jwt generate: no JWT found in terminal output")
	}

	secretOut, err := r.Output(ctx, "podman", "secret", "inspect", "sag-master-key", "--format", "{{.CreatedAt}}")
	if err != nil {
		t.Fatalf("secret inspect sag-master-key: %v", err)
	}
	masterKeyCreatedAt := strings.TrimSpace(string(secretOut))

	ctrOut, err := r.Output(ctx, "podman", "container", "inspect", "sag", "--format", "{{.Id}} {{.State.StartedAt}}")
	if err != nil {
		t.Fatalf("container inspect sag: %v", err)
	}
	ctrState := strings.TrimSpace(string(ctrOut))

	time.Sleep(time.Second)

	bin, err := dxh.TestBinary()
	if err != nil {
		t.Fatalf("second bootstrap: test binary: %v", err)
	}
	out, bootstrapErr := dxh.NewProductSetsidRunner(t).Output(ctx, bin, "bootstrap")
	if bootstrapErr != nil {
		t.Fatalf("second bootstrap: %v\n%s", bootstrapErr, out)
	}

	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "ok ") || strings.HasPrefix(line, "skip tailscale ") {
			continue
		}
		if strings.HasPrefix(line, "https://") || strings.HasPrefix(line, "sag ") {
			continue
		}
		t.Fatalf("second bootstrap: unexpected line %q in output:\n%s", line, out)
	}

	skipCount := strings.Count(string(out), "skip tailscale command absent; loopback only")
	if skipCount != 1 {
		t.Fatalf("second bootstrap: 'skip tailscale command absent; loopback only' count = %d; want 1; output:\n%s", skipCount, out)
	}

	okPortCount := strings.Count(string(out), "ok port-8443")
	if okPortCount != 1 {
		t.Fatalf("second bootstrap: 'ok port-8443' count = %d; want 1; output:\n%s", okPortCount, out)
	}

	secretOut2, err := r.Output(ctx, "podman", "secret", "inspect", "sag-master-key", "--format", "{{.CreatedAt}}")
	if err != nil {
		t.Fatalf("secret inspect sag-master-key after second bootstrap: %v", err)
	}
	if got := strings.TrimSpace(string(secretOut2)); got != masterKeyCreatedAt {
		t.Fatalf("master-key CreatedAt changed: before=%q after=%q", masterKeyCreatedAt, got)
	}

	ctrOut2, err := r.Output(ctx, "podman", "container", "inspect", "sag", "--format", "{{.Id}} {{.State.StartedAt}}")
	if err != nil {
		t.Fatalf("container inspect sag after second bootstrap: %v", err)
	}
	if got := strings.TrimSpace(string(ctrOut2)); got != ctrState {
		t.Fatalf("container state changed after second bootstrap: before=%q after=%q", ctrState, got)
	}

	caPath := filepath.Join(cfgDir, "ca.pem")
	caPEM, err := os.ReadFile(caPath)
	if err != nil {
		t.Fatalf("read ca.pem: %v", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		t.Fatal("ca.pem: no certificate found")
	}
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{RootCAs: pool},
		},
		Timeout: 5 * time.Second,
	}

	garbageReq, _ := http.NewRequestWithContext(ctx, http.MethodPost, "https://localhost:8443/mcp", nil)
	garbageReq.Header.Set("Authorization", "Bearer garbage.invalid.token")
	garbageResp, err := client.Do(garbageReq)
	if err != nil {
		t.Fatalf("POST /mcp with garbage token: %v", err)
	}
	garbageResp.Body.Close()
	if garbageResp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("POST /mcp with garbage token: status = %d; want 401", garbageResp.StatusCode)
	}

	validReq, _ := http.NewRequestWithContext(ctx, http.MethodPost, "https://localhost:8443/mcp", nil)
	validReq.Header.Set("Authorization", "Bearer "+jwt)
	validResp, err := client.Do(validReq)
	if err != nil {
		t.Fatalf("POST /mcp with JWT: %v", err)
	}
	validResp.Body.Close()
	if validResp.StatusCode == http.StatusUnauthorized {
		t.Fatal("POST /mcp with JWT: status 401; JWT not accepted after second bootstrap")
	}
}
