package endpointcert_test

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kanthorlabs/sag/internal/endpointcert"
	"github.com/kanthorlabs/sag/internal/podman"
	"github.com/kanthorlabs/sag/internal/stepline"
)

type fake struct {
	calls []fakeCall
	fn    func(args []string, stdin []byte) ([]byte, error)
}

type fakeCall struct {
	args  []string
	stdin []byte
}

func (f *fake) Run(_ context.Context, stdin io.Reader, args ...string) ([]byte, error) {
	var buf []byte
	if stdin != nil {
		buf, _ = io.ReadAll(stdin)
	}
	f.calls = append(f.calls, fakeCall{args: args, stdin: buf})
	return f.fn(args, buf)
}

func secretsAbsent() func([]string, []byte) ([]byte, error) {
	return func(args []string, _ []byte) ([]byte, error) {
		if len(args) >= 2 && args[0] == "secret" && args[1] == "inspect" {
			return nil, &podman.RunError{ExitCode: 1, Stderr: "no such secret"}
		}
		return nil, nil
	}
}

func secretsPresent() func([]string, []byte) ([]byte, error) {
	return func(args []string, _ []byte) ([]byte, error) {
		return []byte(`[{"ID":"abc"}]`), nil
	}
}

func newNow() time.Time {
	return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
}

func parseCertFromPEM(data []byte) (*x509.Certificate, error) {
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, io.ErrUnexpectedEOF
	}
	return x509.ParseCertificate(block.Bytes)
}

func runEnsure(t *testing.T, fr *fake, cfg endpointcert.Config, now time.Time) (bool, error) {
	t.Helper()
	var buf bytes.Buffer
	out := stepline.New(&buf)
	return endpointcert.Ensure(context.Background(), fr, cfg, now, out)
}

func TestNoNameConstraints(t *testing.T) {
	dir := t.TempDir()
	fr := &fake{fn: secretsAbsent()}
	cfg := endpointcert.Config{CfgDir: dir}
	changed, err := runEnsure(t, fr, cfg, newNow())
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("expected changed=true")
	}

	caPEM, err := os.ReadFile(filepath.Join(dir, "ca.pem"))
	if err != nil {
		t.Fatal(err)
	}
	ca, err := parseCertFromPEM(caPEM)
	if err != nil {
		t.Fatal(err)
	}
	for _, ext := range ca.Extensions {
		if ext.Id.String() == "2.5.29.30" {
			t.Fatal("CA has name constraints extension")
		}
	}
	if !ca.IsCA {
		t.Fatal("CA is not marked as CA")
	}
	if ca.MaxPathLenZero != true {
		t.Fatal("CA MaxPathLenZero is not set")
	}
	if ca.KeyUsage&x509.KeyUsageCertSign == 0 {
		t.Fatal("CA does not have KeyUsageCertSign")
	}
}

func TestServerNames(t *testing.T) {
	dir := t.TempDir()
	fr := &fake{fn: secretsAbsent()}
	cfg := endpointcert.Config{CfgDir: dir}
	_, err := runEnsure(t, fr, cfg, newNow())
	if err != nil {
		t.Fatal(err)
	}

	var certPEM []byte
	for _, c := range fr.calls {
		if len(c.args) >= 3 && c.args[0] == "secret" && c.args[1] == "create" && c.args[2] == "sag-tls-cert" {
			certPEM = c.stdin
		}
	}
	if certPEM == nil {
		t.Fatal("no sag-tls-cert create call")
	}
	cert, err := parseCertFromPEM(certPEM)
	if err != nil {
		t.Fatal(err)
	}

	wantDNS := []string{"localhost"}
	wantIPs := []string{"127.0.0.1", "::1"}

	dnsMap := make(map[string]bool)
	for _, name := range cert.DNSNames {
		dnsMap[name] = true
	}
	ipMap := make(map[string]bool)
	for _, ip := range cert.IPAddresses {
		ipMap[ip.String()] = true
	}

	for _, want := range wantDNS {
		if !dnsMap[want] {
			t.Errorf("missing DNS SAN: %q", want)
		}
	}
	for _, want := range wantIPs {
		if !ipMap[want] {
			t.Errorf("missing IP SAN: %q", want)
		}
	}
	if len(cert.DNSNames) != len(wantDNS) {
		t.Errorf("unexpected DNS SANs: %v", cert.DNSNames)
	}
	if len(cert.IPAddresses) != len(wantIPs) {
		t.Errorf("unexpected IP SANs: %v", cert.IPAddresses)
	}

	hasServerAuth := false
	for _, eku := range cert.ExtKeyUsage {
		if eku == x509.ExtKeyUsageServerAuth {
			hasServerAuth = true
		}
	}
	if !hasServerAuth {
		t.Fatal("server cert does not have ExtKeyUsageServerAuth")
	}
}

func TestTailscaleNameAdded(t *testing.T) {
	dir := t.TempDir()
	fr := &fake{fn: secretsAbsent()}
	cfg := endpointcert.Config{CfgDir: dir, TailscaleIP: "100.64.0.1"}
	_, err := runEnsure(t, fr, cfg, newNow())
	if err != nil {
		t.Fatal(err)
	}

	var certPEM []byte
	for _, c := range fr.calls {
		if len(c.args) >= 3 && c.args[0] == "secret" && c.args[1] == "create" && c.args[2] == "sag-tls-cert" {
			certPEM = c.stdin
		}
	}
	cert, err := parseCertFromPEM(certPEM)
	if err != nil {
		t.Fatal(err)
	}

	found := false
	for _, ip := range cert.IPAddresses {
		if ip.String() == "100.64.0.1" {
			found = true
		}
	}
	if !found {
		t.Fatalf("Tailscale IP not in SANs; got %v", cert.IPAddresses)
	}
	if len(cert.IPAddresses) != 3 {
		t.Fatalf("expected 3 IP SANs (127.0.0.1, ::1, Tailscale), got %v", cert.IPAddresses)
	}
}

func TestSequenceOrder(t *testing.T) {
	dir := t.TempDir()

	type snapshot struct {
		call           string
		caPemNewExists bool
		caPemExists    bool
	}
	var snapshots []snapshot

	fr := &fake{fn: func(args []string, stdin []byte) ([]byte, error) {
		_, errNew := os.Stat(filepath.Join(dir, "ca.pem.new"))
		_, errPem := os.Stat(filepath.Join(dir, "ca.pem"))
		snapshots = append(snapshots, snapshot{
			call:           strings.Join(args, " "),
			caPemNewExists: errNew == nil,
			caPemExists:    errPem == nil,
		})
		if len(args) >= 2 && args[0] == "secret" && args[1] == "inspect" {
			return nil, &podman.RunError{ExitCode: 1}
		}
		return nil, nil
	}}
	cfg := endpointcert.Config{CfgDir: dir}
	_, err := runEnsure(t, fr, cfg, newNow())
	if err != nil {
		t.Fatal(err)
	}

	for _, s := range snapshots {
		if strings.Contains(s.call, "secret rm") || strings.Contains(s.call, "secret create") {
			if !s.caPemNewExists {
				t.Errorf("ca.pem.new must exist before secret rm/create; at call %q", s.call)
			}
		}
	}
	_, errNew := os.Stat(filepath.Join(dir, "ca.pem.new"))
	if errNew == nil {
		t.Fatal("ca.pem.new must be removed after sequence completes")
	}
	_, errPem := os.Stat(filepath.Join(dir, "ca.pem"))
	if errPem != nil {
		t.Fatal("ca.pem must exist after sequence completes")
	}
}

func TestTriggers(t *testing.T) {
	now := newNow()

	t.Run("cert_absent", func(t *testing.T) {
		dir := t.TempDir()
		fr := &fake{fn: func(args []string, _ []byte) ([]byte, error) {
			if len(args) >= 3 && args[0] == "secret" && args[1] == "inspect" && args[2] == "sag-tls-cert" {
				return nil, &podman.RunError{ExitCode: 1}
			}
			return nil, nil
		}}
		changed, err := runEnsure(t, fr, endpointcert.Config{CfgDir: dir}, now)
		if err != nil {
			t.Fatal(err)
		}
		if !changed {
			t.Fatal("expected changed=true when sag-tls-cert absent")
		}
	})

	t.Run("key_absent", func(t *testing.T) {
		dir := t.TempDir()
		fr := &fake{fn: func(args []string, _ []byte) ([]byte, error) {
			if len(args) >= 3 && args[0] == "secret" && args[1] == "inspect" && args[2] == "sag-tls-key" {
				return nil, &podman.RunError{ExitCode: 1}
			}
			return nil, nil
		}}
		changed, err := runEnsure(t, fr, endpointcert.Config{CfgDir: dir}, now)
		if err != nil {
			t.Fatal(err)
		}
		if !changed {
			t.Fatal("expected changed=true when sag-tls-key absent")
		}
	})

	t.Run("ca_pem_absent", func(t *testing.T) {
		dir := t.TempDir()
		fr := &fake{fn: secretsPresent()}
		changed, err := runEnsure(t, fr, endpointcert.Config{CfgDir: dir}, now)
		if err != nil {
			t.Fatal(err)
		}
		if !changed {
			t.Fatal("expected changed=true when ca.pem absent")
		}
	})

	t.Run("ca_pem_new_exists", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "ca.pem.new"), []byte("stale"), 0644); err != nil {
			t.Fatal(err)
		}
		fr := &fake{fn: secretsPresent()}
		changed, err := runEnsure(t, fr, endpointcert.Config{CfgDir: dir}, now)
		if err != nil {
			t.Fatal(err)
		}
		if !changed {
			t.Fatal("expected changed=true when ca.pem.new exists")
		}
	})

	t.Run("tailscale_ip_mismatch", func(t *testing.T) {
		dir := t.TempDir()
		fr := &fake{fn: secretsAbsent()}
		if _, err := runEnsure(t, fr, endpointcert.Config{CfgDir: dir}, now); err != nil {
			t.Fatal(err)
		}
		fr2 := &fake{fn: secretsPresent()}
		changed, err := runEnsure(t, fr2, endpointcert.Config{CfgDir: dir, TailscaleIP: "100.64.0.2"}, now)
		if err != nil {
			t.Fatal(err)
		}
		if !changed {
			t.Fatal("expected changed=true when Tailscale IP changes")
		}
	})

	t.Run("no_trigger_when_all_present", func(t *testing.T) {
		dir := t.TempDir()
		fr := &fake{fn: secretsAbsent()}
		if _, err := runEnsure(t, fr, endpointcert.Config{CfgDir: dir}, now); err != nil {
			t.Fatal(err)
		}
		fr2 := &fake{fn: secretsPresent()}
		changed, err := runEnsure(t, fr2, endpointcert.Config{CfgDir: dir}, now)
		if err != nil {
			t.Fatal(err)
		}
		if changed {
			t.Fatal("expected changed=false when all items present and no trigger")
		}
	})
}

func TestRenewalWithFakeClock(t *testing.T) {
	dir := t.TempDir()
	now := newNow()

	fr := &fake{fn: secretsAbsent()}
	if _, err := runEnsure(t, fr, endpointcert.Config{CfgDir: dir}, now); err != nil {
		t.Fatal(err)
	}

	caPEM, err := os.ReadFile(filepath.Join(dir, "ca.pem"))
	if err != nil {
		t.Fatal(err)
	}
	ca, err := parseCertFromPEM(caPEM)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("not_triggered_at_30_days", func(t *testing.T) {
		dir2 := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir2, "ca.pem"), caPEM, 0644); err != nil {
			t.Fatal(err)
		}
		exactly30 := ca.NotAfter.Add(-30 * 24 * time.Hour)
		fr2 := &fake{fn: secretsPresent()}
		changed, err := runEnsure(t, fr2, endpointcert.Config{CfgDir: dir2}, exactly30)
		if err != nil {
			t.Fatal(err)
		}
		if changed {
			t.Fatal("expected no renewal at exactly 30 days remaining")
		}
	})

	t.Run("triggered_at_29_days", func(t *testing.T) {
		dir3 := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir3, "ca.pem"), caPEM, 0644); err != nil {
			t.Fatal(err)
		}
		under30 := ca.NotAfter.Add(-30*24*time.Hour + time.Second)
		fr3 := &fake{fn: secretsAbsent()}
		changed, err := runEnsure(t, fr3, endpointcert.Config{CfgDir: dir3}, under30)
		if err != nil {
			t.Fatal(err)
		}
		if !changed {
			t.Fatal("expected renewal when less than 30 days remain")
		}
	})
}

func TestKeyNeverInArgsOrFile(t *testing.T) {
	dir := t.TempDir()
	var buf bytes.Buffer
	out := stepline.New(&buf)
	fr := &fake{fn: secretsAbsent()}
	cfg := endpointcert.Config{CfgDir: dir, TailscaleIP: "100.64.0.1"}

	if _, err := endpointcert.Ensure(context.Background(), fr, cfg, newNow(), out); err != nil {
		t.Fatal(err)
	}

	for _, c := range fr.calls {
		for _, arg := range c.args {
			if strings.Contains(arg, "PRIVATE KEY") {
				t.Fatalf("private key material in args: %q", arg)
			}
		}
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		path := filepath.Join(dir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(data, []byte("PRIVATE KEY")) {
			t.Fatalf("private key material in file %q", e.Name())
		}
	}

	if strings.Contains(buf.String(), "PRIVATE KEY") {
		t.Fatal("private key material in stepline output")
	}

	var certPEM, keyPEM []byte
	for _, c := range fr.calls {
		if len(c.args) >= 3 && c.args[0] == "secret" && c.args[1] == "create" {
			switch c.args[2] {
			case "sag-tls-cert":
				certPEM = c.stdin
			case "sag-tls-key":
				keyPEM = c.stdin
			}
		}
	}
	if certPEM == nil || keyPEM == nil {
		t.Fatal("missing cert or key create call")
	}

	if _, err := tls.X509KeyPair(certPEM, keyPEM); err != nil {
		t.Fatalf("cert and key do not match: %v", err)
	}

	cert, err := parseCertFromPEM(certPEM)
	if err != nil {
		t.Fatal(err)
	}
	caPEM, err := os.ReadFile(filepath.Join(dir, "ca.pem"))
	if err != nil {
		t.Fatal(err)
	}
	caPool := x509.NewCertPool()
	caPool.AppendCertsFromPEM(caPEM)
	if _, err := cert.Verify(x509.VerifyOptions{
		Roots:       caPool,
		DNSName:     "localhost",
		CurrentTime: newNow(),
	}); err != nil {
		t.Fatalf("cert chain verification failed: %v", err)
	}

	var found bool
	for _, ip := range cert.IPAddresses {
		if ip.Equal(net.ParseIP("100.64.0.1")) {
			found = true
		}
	}
	if !found {
		t.Fatal("Tailscale IP not in cert SANs")
	}
}
