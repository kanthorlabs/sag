package dxh

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFindsGivenString(t *testing.T) {
	sources := []string{
		"output with password=abc123 in it",
		"another line without sensitive content",
		"third line with myhost in it",
	}
	needles := []string{"abc123", "myhost", "notpresent"}

	found := SearchStrings(sources, needles)
	foundMap := make(map[string]bool)
	for _, f := range found {
		foundMap[f] = true
	}

	if !foundMap["abc123"] {
		t.Error("SearchStrings: expected abc123 to be found")
	}
	if !foundMap["myhost"] {
		t.Error("SearchStrings: expected myhost to be found")
	}
	if foundMap["notpresent"] {
		t.Error("SearchStrings: notpresent must not be found")
	}
}

func TestKeyScanPrintsNoContent(t *testing.T) {
	dir := t.TempDir()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("gen key: %v", err)
	}

	der, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}
	var buf bytes.Buffer
	if err := pem.Encode(&buf, &pem.Block{Type: "EC PRIVATE KEY", Bytes: der}); err != nil {
		t.Fatalf("pem encode: %v", err)
	}
	keyFile := filepath.Join(dir, "leaked.pem")
	if err := os.WriteFile(keyFile, buf.Bytes(), 0o600); err != nil {
		t.Fatalf("write key file: %v", err)
	}

	result, err := ScanForMatchingKey(context.Background(), dir, "", &key.PublicKey, time.Unix(0, 0))
	if err != nil {
		t.Fatalf("ScanForMatchingKey: %v", err)
	}
	if !result.MatchFound {
		t.Fatal("expected match to be found")
	}
	if result.MatchPath == "" {
		t.Fatal("MatchPath must be set when match found")
	}
	if len(result.MatchPath) == 0 {
		t.Fatal("MatchPath must not be empty")
	}

	if bytes.Contains(buf.Bytes(), []byte("EC PRIVATE KEY")) {
		pemContent := string(buf.Bytes())
		if result.MatchPath == pemContent {
			t.Error("ScanForMatchingKey must not return file content in MatchPath")
		}
	}
}

func TestKeyScanSkipsOldFiles(t *testing.T) {
	dir := t.TempDir()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("gen key: %v", err)
	}

	der, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}
	var buf bytes.Buffer
	pem.Encode(&buf, &pem.Block{Type: "EC PRIVATE KEY", Bytes: der})
	keyFile := filepath.Join(dir, "keyfile.pem")
	if err := os.WriteFile(keyFile, buf.Bytes(), 0o600); err != nil {
		t.Fatalf("write key file: %v", err)
	}

	futureStart := time.Now().Add(10 * time.Minute)
	result, err := ScanForMatchingKey(context.Background(), dir, "", &key.PublicKey, futureStart)
	if err != nil {
		t.Fatalf("ScanForMatchingKey with future start: %v", err)
	}
	if result.MatchFound {
		t.Error("file created before groupStart must be skipped")
	}

	pastStart := time.Unix(0, 0)
	result2, err := ScanForMatchingKey(context.Background(), dir, "", &key.PublicKey, pastStart)
	if err != nil {
		t.Fatalf("ScanForMatchingKey with past start: %v", err)
	}
	if !result2.MatchFound {
		t.Error("file should be found when groupStart is in the past")
	}
}

func TestKeyScanCountsDenied(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root; permission checks do not apply")
	}

	dir := t.TempDir()
	deniedDir := filepath.Join(dir, "denied")
	if err := os.MkdirAll(deniedDir, 0o700); err != nil {
		t.Fatalf("mkdir denied: %v", err)
	}

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("gen key: %v", err)
	}
	der, _ := x509.MarshalECPrivateKey(key)
	var buf bytes.Buffer
	pem.Encode(&buf, &pem.Block{Type: "EC PRIVATE KEY", Bytes: der})
	os.WriteFile(filepath.Join(deniedDir, "keyfile.pem"), buf.Bytes(), 0o600)

	if err := os.Chmod(deniedDir, 0o000); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	defer os.Chmod(deniedDir, 0o700)

	result, err := ScanForMatchingKey(context.Background(), dir, "", &key.PublicKey, time.Unix(0, 0))
	if err != nil {
		t.Fatalf("ScanForMatchingKey: %v", err)
	}
	if result.DeniedDirs == 0 {
		t.Error("expected at least one denied directory to be counted")
	}
	if result.MatchFound {
		t.Error("key inside denied directory must not be reported as found")
	}
}
