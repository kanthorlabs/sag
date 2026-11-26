package credential

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"io"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kanthorlabs/sag/internal/core"
	"github.com/kanthorlabs/sag/internal/secret"
)

type failSecondPutStorage struct {
	*fakeStorage
	putCount int
}

func (s *failSecondPutStorage) Put(ctx context.Context, name string, value secret.Credential, lbls map[string]string) error {
	s.putCount++
	if s.putCount == 2 {
		return errors.New("injected CA put failure")
	}
	return s.fakeStorage.Put(ctx, name, value, lbls)
}

func unitSelfSignedPEM(t *testing.T) []byte {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("gen key: %v", err)
	}
	serial, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	now := time.Now()
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: "test CA"},
		NotBefore:    now.Add(-time.Hour),
		NotAfter:     now.Add(24 * time.Hour),
		IsCA:         true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create cert: %v", err)
	}
	var buf bytes.Buffer
	if err := pem.Encode(&buf, &pem.Block{Type: "CERTIFICATE", Bytes: der}); err != nil {
		t.Fatalf("pem encode: %v", err)
	}
	return buf.Bytes()
}

func TestRollbackOnServiceStartFail(t *testing.T) {
	dir := t.TempDir()
	writeSAGYAML(t, dir, baseYAML)

	snapshot, err := os.ReadFile(filepath.Join(dir, "sag.yaml"))
	if err != nil {
		t.Fatalf("read snapshot: %v", err)
	}

	var startCalls int
	store := newFakeStorage()

	opts := CreateOptions{
		Name:         "mydb",
		Kind:         "fakedb",
		Agents:       []string{"agent-a"},
		TLS:          core.TLSDisable,
		CfgDir:       dir,
		Storage:      store,
		KindLookup:   func(k string) (core.CredentialKind, bool) { return fakeKind{}, k == "fakedb" },
		CheckCred:    func(_ context.Context, _ string, _ core.TLSMode, _ secret.Credential, _ []byte) error { return nil },
		ContainerRun: fakeContainerRunning,
		OpenTTY:      func() (*os.File, error) { return nil, errors.New("no tty") },
		StdinReader:  strings.NewReader("test-value-rollback\n"),
		IsStdinTerm:  false,
		Stdin:        true,
		Out:          io.Discard,
		Err:          io.Discard,
	}

	opts.ServiceStart = func(ctx context.Context, out io.Writer) error {
		startCalls++
		if startCalls == 1 {
			return errors.New("service start injected failure")
		}
		return nil
	}

	err = Create(context.Background(), opts)
	if err == nil {
		t.Fatal("want error from service start failure, got nil")
	}

	found := false
	for _, name := range store.deleted {
		if name == "sag-res-mydb.conn" {
			found = true
		}
	}
	if !found {
		t.Errorf("rollback: connection secret not in deleted list; got %v", store.deleted)
	}

	got, readErr := os.ReadFile(filepath.Join(dir, "sag.yaml"))
	if readErr != nil {
		t.Fatalf("read yaml after rollback: %v", readErr)
	}
	if string(got) != string(snapshot) {
		t.Errorf("rollback: sag.yaml not restored\ngot:\n%s\nwant:\n%s", got, snapshot)
	}

	if startCalls != 2 {
		t.Errorf("service start calls = %d; want 2 (one in applyCreate, one in rollback)", startCalls)
	}
}

func TestRollbackOnCAPutFail(t *testing.T) {
	dir := t.TempDir()
	writeSAGYAML(t, dir, baseYAML)

	snapshot, err := os.ReadFile(filepath.Join(dir, "sag.yaml"))
	if err != nil {
		t.Fatalf("read snapshot: %v", err)
	}

	inner := newFakeStorage()
	store := &failSecondPutStorage{fakeStorage: inner}

	opts := CreateOptions{
		Name:         "mydb",
		Kind:         "fakedb",
		Agents:       []string{"agent-a"},
		TLS:          core.TLSDisable,
		TLSCAPem:     unitSelfSignedPEM(t),
		CfgDir:       dir,
		Storage:      store,
		KindLookup:   func(k string) (core.CredentialKind, bool) { return fakeKind{}, k == "fakedb" },
		CheckCred:    func(_ context.Context, _ string, _ core.TLSMode, _ secret.Credential, _ []byte) error { return nil },
		ContainerRun: fakeContainerRunning,
		ServiceStart: noopServiceStart,
		OpenTTY:      func() (*os.File, error) { return nil, errors.New("no tty") },
		StdinReader:  strings.NewReader("test-value-ca-rollback\n"),
		IsStdinTerm:  false,
		Stdin:        true,
		Out:          io.Discard,
		Err:          io.Discard,
	}

	err = Create(context.Background(), opts)
	if err == nil {
		t.Fatal("want error from CA put failure, got nil")
	}

	found := false
	for _, name := range inner.deleted {
		if name == "sag-res-mydb.conn" {
			found = true
		}
	}
	if !found {
		t.Errorf("rollback: connection secret not in deleted list; got %v", inner.deleted)
	}

	got, readErr := os.ReadFile(filepath.Join(dir, "sag.yaml"))
	if readErr != nil {
		t.Fatalf("read yaml after rollback: %v", readErr)
	}
	if string(got) != string(snapshot) {
		t.Errorf("rollback: sag.yaml not restored\ngot:\n%s\nwant:\n%s", got, snapshot)
	}
}
