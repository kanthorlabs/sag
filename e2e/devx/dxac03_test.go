//go:build devxe2e

package devx_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kanthorlabs/sag/e2e/devx/dxh"
)

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
	jwtCode, err := tty.Wait(ctx)
	if err != nil {
		t.Fatalf("jwt generate wait: %v", err)
	}
	if jwtCode != 0 {
		t.Fatalf("jwt generate exit %d; want 0", jwtCode)
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

	out, err := runSAG(t, ctx, "bootstrap")
	if err != nil {
		t.Fatalf("second bootstrap: %v\n%s", err, out)
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
}
