package endpoint

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"io"
	"math/big"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kanthorlabs/sag/internal/token"
)

var testKey = bytes.Repeat([]byte{7}, 32)

func testCertificate(t *testing.T) (tls.Certificate, *x509.CertPool) {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "sag-test"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IsCA:                  true,
		BasicConstraintsValid: true,
		DNSNames:              []string{"localhost"},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &priv.PublicKey, priv)
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(leaf)
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: priv, Leaf: leaf}, pool
}

type testServer struct {
	URL  string
	pool *x509.CertPool
}

func (ts testServer) client(cfg *tls.Config) *http.Client {
	cfg.RootCAs = ts.pool
	return &http.Client{Transport: &http.Transport{TLSClientConfig: cfg}}
}

func startServer(t *testing.T, mcp http.Handler) testServer {
	t.Helper()
	cert, pool := testCertificate(t)
	s, err := New(Options{Listen: "127.0.0.1:0", Certificate: cert, JWTKey: testKey, MCP: mcp})
	if err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.serve(ctx, ln) }()
	t.Cleanup(func() {
		cancel()
		if err := <-done; err != nil {
			t.Errorf("serve: %v", err)
		}
	})
	return testServer{URL: "https://" + ln.Addr().String(), pool: pool}
}

func validToken(t *testing.T, sub string) string {
	t.Helper()
	raw, err := token.Issue(testKey, sub, "Test Agent", time.Now(), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

type recorder struct {
	mu     sync.Mutex
	calls  int
	read   int
	err    error
	status int
}

func (rec *recorder) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	n, err := io.Copy(io.Discard, r.Body)
	rec.mu.Lock()
	rec.calls++
	rec.read = int(n)
	rec.err = err
	rec.mu.Unlock()
	w.WriteHeader(http.StatusOK)
}

func (rec *recorder) snapshot() (int, int, error) {
	rec.mu.Lock()
	defer rec.mu.Unlock()
	return rec.calls, rec.read, rec.err
}

func TestNewRequiresMCPHandlerAndCertificate(t *testing.T) {
	cert, _ := testCertificate(t)
	if _, err := New(Options{Certificate: cert, JWTKey: testKey}); err == nil {
		t.Error("New without MCP handler: want error")
	}
	if _, err := New(Options{JWTKey: testKey, MCP: &recorder{}}); err == nil {
		t.Error("New without certificate: want error")
	}
	if _, err := New(Options{Certificate: cert, MCP: &recorder{}}); err == nil {
		t.Error("New without JWT key: want error")
	}
}

func TestTLS13Minimum(t *testing.T) {
	ts := startServer(t, &recorder{})

	old := ts.client(&tls.Config{MaxVersion: tls.VersionTLS12})
	if resp, err := old.Get(ts.URL + "/healthz"); err == nil {
		resp.Body.Close()
		t.Fatal("TLS 1.2 client: want handshake failure")
	}

	resp, err := ts.client(&tls.Config{}).Get(ts.URL + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.TLS == nil || resp.TLS.Version != tls.VersionTLS13 {
		t.Fatalf("TLS version: got %v, want TLS 1.3", resp.TLS)
	}
}

func TestServerTimeouts(t *testing.T) {
	cert, _ := testCertificate(t)
	s, err := New(Options{Certificate: cert, JWTKey: testKey, MCP: &recorder{}})
	if err != nil {
		t.Fatal(err)
	}
	srv := s.httpServer()
	if srv.ReadHeaderTimeout != 5*time.Second {
		t.Errorf("ReadHeaderTimeout: got %v, want 5s", srv.ReadHeaderTimeout)
	}
	if srv.TLSConfig.MinVersion != tls.VersionTLS13 {
		t.Errorf("MinVersion: got %x, want TLS 1.3", srv.TLSConfig.MinVersion)
	}
}

func TestBodyLimit64KiB(t *testing.T) {
	rec := &recorder{}
	ts := startServer(t, rec)
	client := ts.client(&tls.Config{})
	bearer := "Bearer " + validToken(t, "ac03")

	post := func(body io.Reader, length int64) int {
		t.Helper()
		req, err := http.NewRequest(http.MethodPost, ts.URL+"/mcp", body)
		if err != nil {
			t.Fatal(err)
		}
		req.ContentLength = length
		req.Header.Set("Authorization", bearer)
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}

	if got := post(bytes.NewReader(make([]byte, 64<<10)), 64<<10); got != http.StatusOK {
		t.Fatalf("64 KiB body: got %d, want 200", got)
	}
	if _, read, err := rec.snapshot(); read != 64<<10 || err != nil {
		t.Fatalf("64 KiB body: handler read %d bytes, err %v", read, err)
	}

	if got := post(bytes.NewReader(make([]byte, 64<<10+1)), 64<<10+1); got != http.StatusRequestEntityTooLarge {
		t.Fatalf("64 KiB + 1 body: got %d, want 413", got)
	}
	if calls, _, _ := rec.snapshot(); calls != 1 {
		t.Fatalf("64 KiB + 1 body: handler calls %d, want 1", calls)
	}

	post(io.MultiReader(bytes.NewReader(make([]byte, 64<<10+1))), -1)
	_, read, err := rec.snapshot()
	var tooLarge *http.MaxBytesError
	if !errors.As(err, &tooLarge) || read > 64<<10 {
		t.Fatalf("chunked body: handler read %d bytes, err %v, want MaxBytesError", read, err)
	}
}

func TestHealthzNoToken(t *testing.T) {
	rec := &recorder{}
	ts := startServer(t, rec)
	resp, err := ts.client(&tls.Config{}).Get(ts.URL + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || len(body) != 0 {
		t.Fatalf("GET /healthz: got %d %q, want 200 and empty body", resp.StatusCode, body)
	}
	if calls, _, _ := rec.snapshot(); calls != 0 {
		t.Fatalf("GET /healthz reached the MCP handler")
	}

	resp, err = ts.client(&tls.Config{}).Post(ts.URL+"/healthz", "text/plain", strings.NewReader("x"))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("POST /healthz without token: got %d, want 401", resp.StatusCode)
	}
}
