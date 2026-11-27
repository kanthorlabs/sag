package agentsetup

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func generateTestCert(t *testing.T) []byte {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test-ca"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		IsCA:         true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}

func TestAcceptsSameCert(t *testing.T) {
	dir := t.TempDir()
	certPEM := generateTestCert(t)
	caPath := filepath.Join(dir, "ca.pem")
	os.WriteFile(caPath, certPEM, 0o600)

	if !acceptsCATrust(caPath, caPath) {
		t.Error("acceptsCATrust(same path) = false; want true")
	}
}

func TestBundleCommand(t *testing.T) {
	dir := t.TempDir()
	otherPEM := generateTestCert(t)
	caPath := filepath.Join(dir, "ca.pem")
	os.WriteFile(caPath, generateTestCert(t), 0o600)

	otherPath := filepath.Join(dir, "other.pem")
	os.WriteFile(otherPath, otherPEM, 0o600)

	err := bundleConflictError(otherPath, caPath, dir+"/settings.json")
	if err == nil {
		t.Fatal("bundleConflictError returned nil")
	}
	msg := err.Error()
	if !containsString(msg, "cat") {
		t.Errorf("bundle error message missing 'cat': %q", msg)
	}
	if !containsString(msg, "NODE_EXTRA_CA_CERTS") {
		t.Errorf("bundle error message missing 'NODE_EXTRA_CA_CERTS': %q", msg)
	}
}
