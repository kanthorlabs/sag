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
	"strings"
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

func TestB94BundleLineFromEnv(t *testing.T) {
	dir := t.TempDir()
	caPath := filepath.Join(dir, "ca.pem")
	otherPath := filepath.Join(dir, "other.pem")
	os.WriteFile(caPath, generateTestCert(t), 0o600)
	os.WriteFile(otherPath, generateTestCert(t), 0o600)

	err := bundleConflictError(otherPath, caPath, "the environment")
	if err == nil {
		t.Fatal("B94: want error")
	}
	msg := err.Error()
	wantPrefix := "fail ca-trust NODE_EXTRA_CA_CERTS is " + otherPath
	if !strings.HasPrefix(msg, wantPrefix) {
		t.Errorf("B94 env: message = %q; want prefix %q", msg, wantPrefix)
	}
	if !strings.Contains(msg, "in the environment") {
		t.Errorf("B94 env: message = %q; want 'in the environment'", msg)
	}
}

func TestB94BundleLineFromSettings(t *testing.T) {
	dir := t.TempDir()
	caPath := filepath.Join(dir, "ca.pem")
	otherPath := filepath.Join(dir, "other.pem")
	settingsPath := filepath.Join(dir, "settings.json")
	os.WriteFile(caPath, generateTestCert(t), 0o600)
	os.WriteFile(otherPath, generateTestCert(t), 0o600)

	err := bundleConflictError(otherPath, caPath, settingsPath)
	if err == nil {
		t.Fatal("B94: want error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "in "+settingsPath) {
		t.Errorf("B94 settings: message = %q; want 'in %s'", msg, settingsPath)
	}
}

func TestB94BundleLineFromProfile(t *testing.T) {
	dir := t.TempDir()
	caPath := filepath.Join(dir, "ca.pem")
	otherPath := filepath.Join(dir, "other.pem")
	profilePath := filepath.Join(dir, ".zshrc")
	os.WriteFile(caPath, generateTestCert(t), 0o600)
	os.WriteFile(otherPath, generateTestCert(t), 0o600)

	err := bundleConflictError(otherPath, caPath, profilePath)
	if err == nil {
		t.Fatal("B94: want error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "in "+profilePath) {
		t.Errorf("B94 profile: message = %q; want 'in %s'", msg, profilePath)
	}
}

func TestB88ProfileForeignNECCConflict(t *testing.T) {
	dir := t.TempDir()
	profile := filepath.Join(dir, ".zshrc")
	caPath := filepath.Join(dir, "ca.pem")
	os.WriteFile(caPath, generateTestCert(t), 0o600)

	foreignPath := filepath.Join(dir, "foreign.pem")
	os.WriteFile(foreignPath, generateTestCert(t), 0o600)
	os.WriteFile(profile, []byte("export NODE_EXTRA_CA_CERTS=\""+foreignPath+"\"\n"), 0o600)

	opts := Options{
		Harness: HarnessPi,
		CfgDir:  dir,
		GOOS:    "darwin",
		Home:    dir,
		Env:     map[string]string{"SHELL": "/bin/zsh"},
	}
	paths := PiPaths("", dir)
	c := checkCATrustConflict(opts, paths)
	if c == "" {
		t.Fatal("B88: want conflict for foreign NODE_EXTRA_CA_CERTS in profile")
	}
	if !strings.Contains(c, "NODE_EXTRA_CA_CERTS") {
		t.Errorf("B88: conflict = %q; want NODE_EXTRA_CA_CERTS mention", c)
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
	if !strings.Contains(msg, "cat") {
		t.Errorf("bundle error message missing 'cat': %q", msg)
	}
	if !strings.Contains(msg, "NODE_EXTRA_CA_CERTS") {
		t.Errorf("bundle error message missing 'NODE_EXTRA_CA_CERTS': %q", msg)
	}
}
