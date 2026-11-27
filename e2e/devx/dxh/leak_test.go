package dxh

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
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

func TestSearchNamedParts(t *testing.T) {
	sources := map[string]string{
		"log output":     "starting up with password=secret123",
		"inspect output": "host=myhost.example.com info",
		"clean output":   "nothing sensitive here",
	}
	parts := map[string]string{
		"password": "secret123",
		"host":     "myhost.example.com",
		"user":     "myuser99",
		"database": "mydb01",
	}

	findings := SearchNamedParts(sources, parts)

	foundMap := make(map[string]bool)
	for _, f := range findings {
		foundMap[f] = true
	}

	if !foundMap["password in log output"] {
		t.Error("expected finding: password in log output")
	}
	if !foundMap["host in inspect output"] {
		t.Error("expected finding: host in inspect output")
	}
	for _, f := range findings {
		if f == "user in log output" || f == "database in log output" {
			t.Errorf("unexpected finding: %s", f)
		}
	}
	for _, f := range findings {
		if f == "secret123" || f == "myhost.example.com" {
			t.Errorf("finding must not contain the value: %s", f)
		}
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

	oldStdout := os.Stdout
	oldStderr := os.Stderr
	rOut, wOut, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe stdout: %v", err)
	}
	rErr, wErr, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe stderr: %v", err)
	}
	os.Stdout = wOut
	os.Stderr = wErr

	result, scanErr := ScanForMatchingKey(context.Background(), dir, "", &key.PublicKey, time.Unix(0, 0))

	wOut.Close()
	wErr.Close()
	os.Stdout = oldStdout
	os.Stderr = oldStderr

	var capturedOut, capturedErr bytes.Buffer
	io.Copy(&capturedOut, rOut)
	io.Copy(&capturedErr, rErr)
	rOut.Close()
	rErr.Close()

	if capturedOut.Len() > 0 {
		t.Errorf("ScanForMatchingKey must not write to stdout; got %d bytes", capturedOut.Len())
	}
	if capturedErr.Len() > 0 {
		t.Errorf("ScanForMatchingKey must not write to stderr; got %d bytes", capturedErr.Len())
	}
	if scanErr != nil {
		t.Fatalf("ScanForMatchingKey: %v", scanErr)
	}
	if !result.MatchFound {
		t.Fatal("expected match to be found")
	}
	if strings.Contains(result.MatchPath, "EC PRIVATE KEY") {
		t.Error("MatchPath must not contain key content")
	}
}

func TestKeyScanSkipsFileSymlink(t *testing.T) {
	dir := t.TempDir()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("gen key: %v", err)
	}

	der, _ := x509.MarshalECPrivateKey(key)
	var buf bytes.Buffer
	pem.Encode(&buf, &pem.Block{Type: "EC PRIVATE KEY", Bytes: der})

	realFile := filepath.Join(dir, "real.pem")
	if err := os.WriteFile(realFile, buf.Bytes(), 0o600); err != nil {
		t.Fatalf("write key file: %v", err)
	}

	linkFile := filepath.Join(dir, "link.pem")
	if err := os.Symlink(realFile, linkFile); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	if err := os.Remove(realFile); err != nil {
		t.Fatalf("remove real file: %v", err)
	}

	result, err := ScanForMatchingKey(context.Background(), dir, "", &key.PublicKey, time.Unix(0, 0))
	if err != nil {
		t.Fatalf("ScanForMatchingKey: %v", err)
	}
	if result.MatchFound {
		t.Error("key behind a symlink must not be found")
	}
}

func TestKeyScanSkipsSymlinkRoot(t *testing.T) {
	realDir := t.TempDir()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("gen key: %v", err)
	}

	der, _ := x509.MarshalECPrivateKey(key)
	var buf bytes.Buffer
	pem.Encode(&buf, &pem.Block{Type: "EC PRIVATE KEY", Bytes: der})

	if err := os.WriteFile(filepath.Join(realDir, "key.pem"), buf.Bytes(), 0o600); err != nil {
		t.Fatalf("write key file: %v", err)
	}

	linkParent := t.TempDir()
	linkPath := filepath.Join(linkParent, "symdir")
	if err := os.Symlink(realDir, linkPath); err != nil {
		t.Fatalf("symlink dir: %v", err)
	}

	result, err := ScanForMatchingKey(context.Background(), linkPath, "", &key.PublicKey, time.Unix(0, 0))
	if err != nil {
		t.Fatalf("ScanForMatchingKey: %v", err)
	}
	if !result.MatchFound {
		t.Error("key inside a symlinked root must be found after EvalSymlinks")
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

func TestKeyScanLimitsBySize(t *testing.T) {
	dir := t.TempDir()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("gen key: %v", err)
	}

	der, _ := x509.MarshalECPrivateKey(key)
	var pemBuf bytes.Buffer
	pem.Encode(&pemBuf, &pem.Block{Type: "EC PRIVATE KEY", Bytes: der})
	pemBytes := pemBuf.Bytes()

	padding := make([]byte, int(keyScanMaxSize)-len(pemBytes))
	for i := range padding {
		padding[i] = '\n'
	}
	atLimitBytes := append(append([]byte(nil), pemBytes...), padding...)
	if int64(len(atLimitBytes)) != keyScanMaxSize {
		t.Fatalf("size preparation: got %d, want %d", len(atLimitBytes), keyScanMaxSize)
	}

	atLimitFile := filepath.Join(dir, "at_limit.pem")
	if err := os.WriteFile(atLimitFile, atLimitBytes, 0o600); err != nil {
		t.Fatalf("write at-limit file: %v", err)
	}

	pastStart := time.Unix(0, 0)
	resultAt, err := ScanForMatchingKey(context.Background(), dir, "", &key.PublicKey, pastStart)
	if err != nil {
		t.Fatalf("ScanForMatchingKey at limit: %v", err)
	}
	if !resultAt.MatchFound {
		t.Error("key in file at 1 MiB limit must be found")
	}

	if err := os.Remove(atLimitFile); err != nil {
		t.Fatalf("remove at-limit file: %v", err)
	}

	overLimitBytes := append(append([]byte(nil), atLimitBytes...), '\n')
	overLimitFile := filepath.Join(dir, "over_limit.pem")
	if err := os.WriteFile(overLimitFile, overLimitBytes, 0o600); err != nil {
		t.Fatalf("write over-limit file: %v", err)
	}

	resultOver, err := ScanForMatchingKey(context.Background(), dir, "", &key.PublicKey, pastStart)
	if err != nil {
		t.Fatalf("ScanForMatchingKey over limit: %v", err)
	}
	if resultOver.MatchFound {
		t.Error("key in file above 1 MiB must not be found")
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

func TestKeyScanTimesOut(t *testing.T) {
	dir := t.TempDir()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("gen key: %v", err)
	}

	origTimeout := keyScanTimeout
	keyScanTimeout = 1 * time.Nanosecond
	defer func() { keyScanTimeout = origTimeout }()

	result, err := ScanForMatchingKey(context.Background(), dir, "", &key.PublicKey, time.Unix(0, 0))
	if err == nil {
		t.Fatal("expected error from timed-out scan")
	}
	if !result.TimedOut {
		t.Errorf("expected TimedOut=true; err=%v", err)
	}
}

func TestKeyScanStaysOnFilesystem(t *testing.T) {
	homeDir := t.TempDir()
	alienDir := t.TempDir()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("gen key: %v", err)
	}
	der, _ := x509.MarshalECPrivateKey(key)
	var buf bytes.Buffer
	pem.Encode(&buf, &pem.Block{Type: "EC PRIVATE KEY", Bytes: der})

	keyFile := filepath.Join(alienDir, "key.pem")
	if err := os.WriteFile(keyFile, buf.Bytes(), 0o600); err != nil {
		t.Fatalf("write key file: %v", err)
	}

	homeSt, err := os.Stat(homeDir)
	if err != nil {
		t.Fatalf("stat homeDir: %v", err)
	}
	realDev := int64(homeSt.Sys().(*syscall.Stat_t).Dev)

	alienSt, err := os.Stat(alienDir)
	if err != nil {
		t.Fatalf("stat alienDir: %v", err)
	}
	alienIno := alienSt.Sys().(*syscall.Stat_t).Ino

	keyFileSt, err := os.Stat(keyFile)
	if err != nil {
		t.Fatalf("stat keyFile: %v", err)
	}
	keyFileIno := keyFileSt.Sys().(*syscall.Stat_t).Ino

	origDevOf := keyScanDevOf
	keyScanDevOf = func(s *syscall.Stat_t) int64 {
		if s.Ino == alienIno || s.Ino == keyFileIno {
			return realDev + 1
		}
		return realDev
	}
	defer func() { keyScanDevOf = origDevOf }()

	result, err := ScanForMatchingKey(context.Background(), homeDir, alienDir, &key.PublicKey, time.Unix(0, 0))
	if err != nil {
		t.Fatalf("ScanForMatchingKey: %v", err)
	}
	if result.MatchFound {
		t.Error("key on a different device must not be found")
	}
}
