//go:build devxe2e

package devx_test

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kanthorlabs/sag/e2e/devx/dxh"
)

func DXAC02_ContainerAndCert(t *testing.T, ctx context.Context) {
	t.Helper()
	dxh.EnsureBootstrapped(t)

	r := dxh.NewRunner()

	out, err := r.Output(ctx, "podman", "exec", "sag", "sag", "config", "check")
	if err != nil {
		t.Fatalf("podman exec sag config check: %v\n%s", err, out)
	}

	out, err = r.Output(ctx, "podman", "container", "inspect", "sag", "--format", "{{.HostConfig.LogConfig.Type}}")
	if err != nil {
		t.Fatalf("podman inspect log driver: %v", err)
	}
	if !strings.Contains(string(out), "k8s-file") {
		t.Fatalf("log driver = %q; want k8s-file", strings.TrimSpace(string(out)))
	}

	home := os.Getenv("HOME")
	caPath := filepath.Join(dxh.CfgDir(home), "ca.pem")
	caBytes, err := os.ReadFile(caPath)
	if err != nil {
		t.Fatalf("read ca.pem: %v", err)
	}
	block, _ := pem.Decode(caBytes)
	if block == nil {
		t.Fatal("ca.pem: no PEM block found")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("ca.pem: parse certificate: %v", err)
	}
	if cert.PermittedDNSDomains != nil || cert.ExcludedDNSDomains != nil ||
		cert.PermittedIPRanges != nil || cert.ExcludedIPRanges != nil ||
		cert.PermittedEmailAddresses != nil || cert.ExcludedEmailAddresses != nil ||
		cert.PermittedURIDomains != nil || cert.ExcludedURIDomains != nil {
		t.Fatal("ca.pem has name constraints extension")
	}

	out, err = r.Output(ctx, "podman", "container", "inspect", "sag", "--format",
		"{{range .HostConfig.PortBindings}}{{range .}}{{.HostIp}}:{{.HostPort}}{{end}}{{end}}")
	if err != nil {
		t.Fatalf("podman inspect port bindings: %v", err)
	}
	binding := strings.TrimSpace(string(out))
	if !strings.Contains(binding, "127.0.0.1") {
		t.Fatalf("port binding = %q; want 127.0.0.1", binding)
	}
	fmt.Println("DXAC02 container and cert checks passed")
}
