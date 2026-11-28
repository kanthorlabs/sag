//go:build devxe2e

package devx_test

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/asn1"
	"encoding/json"
	"encoding/pem"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"testing"

	"github.com/kanthorlabs/sag/e2e/devx/dxh"
	"github.com/kanthorlabs/sag/internal/config"
	"github.com/kanthorlabs/sag/internal/deploy"
	"github.com/kanthorlabs/sag/internal/secret"
)

var oidNameConstraints = asn1.ObjectIdentifier{2, 5, 29, 30}

type inspectResult struct {
	HostConfig struct {
		LogConfig struct {
			Type string `json:"Type"`
			Size string `json:"Size"`
		} `json:"LogConfig"`
	} `json:"HostConfig"`
	Config struct {
		Secrets []struct {
			Name string `json:"Name"`
		} `json:"Secrets"`
	} `json:"Config"`
}

func DXAC02_ContainerAndCert(t *testing.T, ctx context.Context) {
	t.Helper()
	dxh.EnsureBootstrapped(t)

	r := dxh.NewRunner()

	out, err := r.Output(ctx, "podman", "exec", "sag", "sag", "config", "check")
	if err != nil {
		t.Fatalf("podman exec sag config check: %v\n%s", err, out)
	}

	out, err = r.Output(ctx, "podman", "container", "inspect", "sag")
	if err != nil {
		t.Fatalf("podman container inspect sag: %v", err)
	}
	var inspected []inspectResult
	if err := json.Unmarshal(out, &inspected); err != nil {
		t.Fatalf("podman container inspect parse: %v", err)
	}
	if len(inspected) != 1 {
		t.Fatalf("podman container inspect returned %d entries; want 1", len(inspected))
	}
	c := inspected[0]

	if c.HostConfig.LogConfig.Type != "k8s-file" {
		t.Fatal("log driver is not k8s-file")
	}
	if c.HostConfig.LogConfig.Size != "10MB" {
		t.Fatal("log max-size is not 10MB")
	}

	home := os.Getenv("HOME")
	cfgDir := dxh.CfgDir(home)
	_, err = config.Load(filepath.Join(cfgDir, "sag.yaml"))
	if err != nil {
		t.Fatalf("load <cfg>/sag.yaml: %v", err)
	}

	wantSecretNames := []string{secret.MasterKeyName, "sag-tls-cert", "sag-tls-key"}
	gotSecrets := len(c.Config.Secrets)
	if gotSecrets != len(wantSecretNames) {
		t.Fatalf("secret count = %d; want %d", gotSecrets, len(wantSecretNames))
	}
	secretSet := map[string]bool{}
	for _, s := range c.Config.Secrets {
		secretSet[s.Name] = true
	}
	for _, name := range wantSecretNames {
		if !secretSet[name] {
			t.Errorf("secret %q not found in container inspect; got %v", name, c.Config.Secrets)
		}
	}

	root := repoRoot(t)
	devCfg, err := config.Load(filepath.Join(root, "deploy", "dev", "sag.yaml"))
	if err != nil {
		t.Fatalf("load deploy/dev/sag.yaml: %v", err)
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

	caPath := filepath.Join(cfgDir, "ca.pem")
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

	caPool := x509.NewCertPool()
	caPool.AppendCertsFromPEM(caBytes)
	conn, err := tls.Dial("tcp", "127.0.0.1:8443", &tls.Config{
		RootCAs:    caPool,
		ServerName: "localhost",
	})
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

	if runtime.GOOS == "linux" {
		if !quadletExists(home) {
			t.Fatal("Quadlet file missing after bootstrap (phase B)")
		}
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
