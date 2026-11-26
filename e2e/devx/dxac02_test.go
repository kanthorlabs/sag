//go:build devxe2e

package devx_test

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/asn1"
	"encoding/pem"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/kanthorlabs/sag/e2e/devx/dxh"
	"github.com/kanthorlabs/sag/internal/config"
	"github.com/kanthorlabs/sag/internal/deploy"
)

var oidNameConstraints = asn1.ObjectIdentifier{2, 5, 29, 30}

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
	if strings.TrimSpace(string(out)) != "k8s-file" {
		t.Fatal("log driver is not k8s-file")
	}

	out, err = r.Output(ctx, "podman", "container", "inspect", "sag",
		"--format", "{{index .HostConfig.LogConfig.Config \"max-size\"}}")
	if err != nil {
		t.Fatalf("podman inspect log max-size: %v", err)
	}
	if strings.TrimSpace(string(out)) != "10mb" {
		t.Fatal("log max-size is not 10mb")
	}

	root := repoRoot(t)

	devCfg, err := config.Load(filepath.Join(root, "deploy", "dev", "sag.yaml"))
	if err != nil {
		t.Fatalf("load deploy/dev/sag.yaml: %v", err)
	}

	wantSecrets := len(deploy.SecretMounts(devCfg))
	out, err = r.Output(ctx, "podman", "container", "inspect", "sag", "--format", "{{len .Secrets}}")
	if err != nil {
		t.Fatalf("podman inspect secrets: %v", err)
	}
	gotSecrets := 0
	for _, c := range strings.TrimSpace(string(out)) {
		if c >= '0' && c <= '9' {
			gotSecrets = gotSecrets*10 + int(c-'0')
		}
	}
	if gotSecrets != wantSecrets {
		t.Fatalf("secret count = %d; want %d", gotSecrets, wantSecrets)
	}

	golden, err := os.ReadFile(filepath.Join(root, "deploy", "sag.container"))
	if err != nil {
		t.Fatalf("read deploy/sag.container: %v", err)
	}
	got := deploy.Quadlet(devCfg, deploy.RunOpts{
		Image:         "localhost/sag:latest",
		ConfigSource:  "/etc/sag/sag.yaml",
		Autostart:     true,
		RestartPolicy: "on-failure",
	})
	if got != string(golden) {
		t.Fatal("Quadlet output for deploy/dev/sag.yaml does not match deploy/sag.container")
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
	caCert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("ca.pem: parse certificate: %v", err)
	}
	for _, ext := range caCert.Extensions {
		if ext.Id.Equal(oidNameConstraints) {
			t.Fatal("ca.pem has name constraints extension")
		}
	}

	tlsCfg := &tls.Config{
		InsecureSkipVerify: false,
		RootCAs: func() *x509.CertPool {
			pool := x509.NewCertPool()
			pool.AppendCertsFromPEM(caBytes)
			return pool
		}(),
		ServerName: "localhost",
	}
	conn, err := tls.Dial("tcp", "127.0.0.1:8443", tlsCfg)
	if err != nil {
		t.Fatalf("tls dial 127.0.0.1:8443: %v", err)
	}
	conn.Close()
	serverCert := conn.ConnectionState().PeerCertificates[0]

	wantDNS := []string{"localhost"}
	wantIPs := []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")}
	sort.Slice(serverCert.IPAddresses, func(i, j int) bool {
		return serverCert.IPAddresses[i].String() < serverCert.IPAddresses[j].String()
	})
	sort.Slice(wantIPs, func(i, j int) bool {
		return wantIPs[i].String() < wantIPs[j].String()
	})

	if len(serverCert.DNSNames) != len(wantDNS) || serverCert.DNSNames[0] != wantDNS[0] {
		t.Fatal("server cert DNS SANs do not match expected names")
	}
	if len(serverCert.IPAddresses) != len(wantIPs) {
		t.Fatal("server cert IP SANs count does not match")
	}
	for i, ip := range wantIPs {
		if !serverCert.IPAddresses[i].Equal(ip) {
			t.Fatal("server cert IP SANs do not match expected addresses")
		}
	}

	out, err = r.Output(ctx, "podman", "container", "inspect", "sag", "--format",
		"{{range .HostConfig.PortBindings}}{{range .}}{{.HostIp}}:{{.HostPort}}{{end}}{{end}}")
	if err != nil {
		t.Fatalf("podman inspect port bindings: %v", err)
	}
	binding := strings.TrimSpace(string(out))
	if !strings.HasPrefix(binding, "127.0.0.1:") {
		t.Fatal("port binding host IP is not 127.0.0.1")
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		t.Fatalf("git rev-parse --show-toplevel: %v", err)
	}
	return string(bytes.TrimSpace(out))
}
