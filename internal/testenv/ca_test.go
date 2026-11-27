package testenv

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"testing"
)

func TestIssueVerifiesAgainstCA(t *testing.T) {
	ca, err := NewCA()
	if err != nil {
		t.Fatal(err)
	}
	certPEM, keyPEM, err := ca.Issue("sag-e2e-abc-pg17", "localhost")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tls.X509KeyPair(certPEM, keyPEM); err != nil {
		t.Fatalf("key pair: %v", err)
	}
	block, _ := pem.Decode(certPEM)
	if block == nil {
		t.Fatal("no PEM block in certificate")
	}
	leaf, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(ca.CertPEM) {
		t.Fatal("CA PEM not accepted")
	}
	for _, host := range []string{"sag-e2e-abc-pg17", "localhost"} {
		opts := x509.VerifyOptions{Roots: roots, DNSName: host, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
		if _, err := leaf.Verify(opts); err != nil {
			t.Errorf("verify %s: %v", host, err)
		}
	}
	opts := x509.VerifyOptions{Roots: roots, DNSName: "other.example"}
	if _, err := leaf.Verify(opts); err == nil {
		t.Error("certificate verified for a host outside the SAN list")
	}
}

func TestIssueFromOtherCAFails(t *testing.T) {
	a, err := NewCA()
	if err != nil {
		t.Fatal(err)
	}
	b, err := NewCA()
	if err != nil {
		t.Fatal(err)
	}
	certPEM, _, err := b.Issue("localhost")
	if err != nil {
		t.Fatal(err)
	}
	block, _ := pem.Decode(certPEM)
	leaf, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	roots.AppendCertsFromPEM(a.CertPEM)
	if _, err := leaf.Verify(x509.VerifyOptions{Roots: roots, DNSName: "localhost"}); err == nil {
		t.Error("certificate verified against a foreign CA")
	}
}
