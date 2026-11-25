package mcpserver

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"

	"github.com/kanthorlabs/sag/internal/endpoint"
	"github.com/kanthorlabs/sag/internal/sql"
	"github.com/kanthorlabs/sag/internal/token"
)

const (
	protocolNew = "2026-07-28"
	metaNew     = `"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientCapabilities":{},"io.modelcontextprotocol/clientInfo":{"name":"test","version":"1"}}`
	listenBody  = `{"jsonrpc":"2.0","id":1,"method":"subscriptions/listen","params":{"notifications":{"toolsListChanged":true},` + metaNew + `}}`
)

func postNew(t *testing.T, client *http.Client, url, bearer, method, body string) (int, string) {
	t.Helper()
	status, out, err := doNew(client, url, bearer, method, body)
	if err != nil {
		t.Fatalf("%s: %v", method, err)
	}
	return status, out
}

func doNew(client *http.Client, url, bearer, method, body string) (int, string, error) {
	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(body))
	if err != nil {
		return 0, "", err
	}
	req.Header.Set("Authorization", "Bearer "+bearer)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Mcp-Protocol-Version", protocolNew)
	req.Header.Set("Mcp-Method", method)
	resp, err := client.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()
	out, err := io.ReadAll(resp.Body)
	return resp.StatusCode, string(out), err
}

func TestToolsCapabilityWithoutListChanged(t *testing.T) {
	withCapabilities(t, sql.Capability())
	calls := &recordingCaller{}
	ts := httptest.NewServer(auth.RequireBearerToken(subAsToken, nil)(Handler(newServer(&laxInventory{}, calls, "test"))))
	t.Cleanup(ts.Close)
	client := &http.Client{Timeout: 3 * time.Second}

	status, body := postNew(t, client, ts.URL, "ac26", "server/discover", `{"jsonrpc":"2.0","id":1,"method":"server/discover","params":{`+metaNew+`}}`)
	if status != http.StatusOK || !strings.Contains(body, `"tools":{}`) || strings.Contains(body, "listChanged") {
		t.Fatalf("server/discover: HTTP %d %s; want a tools capability without listChanged", status, body)
	}

	start := time.Now()
	status, body = postNew(t, client, ts.URL, "ac26", "subscriptions/listen", listenBody)
	if status != http.StatusOK || time.Since(start) > time.Second {
		t.Fatalf("subscriptions/listen: HTTP %d after %s %s; want an immediate response", status, time.Since(start), body)
	}
	if strings.Contains(body, `"toolsListChanged":true`) {
		t.Fatalf("subscriptions/listen: %s; want no toolsListChanged subscription", body)
	}
	if n := calls.audited(); n != 0 {
		t.Fatalf("records = %d; want none", n)
	}
}

func TestRunStopsWithListenClient(t *testing.T) {
	withCapabilities(t, sql.Capability())
	key := bytes.Repeat([]byte{7}, 32)
	cert, pool := selfSignedCertificate(t)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	if err := ln.Close(); err != nil {
		t.Fatal(err)
	}
	srv, err := endpoint.New(endpoint.Options{
		Listen:      addr,
		Certificate: cert,
		JWTKey:      key,
		MCP:         Handler(newServer(&laxInventory{}, &recordingCaller{}, "test")),
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- srv.Run(ctx) }()

	client := &http.Client{Timeout: 3 * time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool}}}
	url := "https://" + addr
	waitHealthy(t, client, url)
	jwt, err := token.Issue(key, "ac26", "Test Agent", time.Now(), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	streamClient := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool}}}
	listened := make(chan error, 1)
	go func() {
		status, _, err := doNew(streamClient, url+"/mcp", jwt, "subscriptions/listen", listenBody)
		if err == nil && status != http.StatusOK {
			err = fmt.Errorf("HTTP %d", status)
		}
		listened <- err
	}()
	select {
	case err := <-listened:
		if err != nil {
			t.Fatalf("subscriptions/listen: %v; want HTTP 200", err)
		}
	case <-time.After(time.Second):
	}

	start := time.Now()
	cancel()
	select {
	case err := <-done:
		if err != nil || time.Since(start) > time.Second {
			t.Fatalf("Run returned %v after %s; want nil at once", err, time.Since(start))
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return 5s after the stop signal")
	}
}

func waitHealthy(t *testing.T, client *http.Client, url string) {
	t.Helper()
	for range 100 {
		resp, err := client.Get(url + "/healthz")
		if err == nil {
			resp.Body.Close()
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("%s/healthz did not respond", url)
}

func selfSignedCertificate(t *testing.T) (tls.Certificate, *x509.CertPool) {
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
