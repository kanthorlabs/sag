package harness

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"regexp"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const httpTimeout = 60 * time.Second

var jwtPattern = regexp.MustCompile(`eyJ[A-Za-z0-9_-]*\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+`)

func newHTTPClient(caPEM []byte) (*http.Client, error) {
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(caPEM) {
		return nil, fmt.Errorf("harness: test CA PEM has no certificate")
	}
	return &http.Client{
		Timeout: httpTimeout,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12},
		},
	}, nil
}

type bearerTransport struct {
	token string
	base  http.RoundTripper
}

func (b bearerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	clone.Header.Set("Authorization", "Bearer "+b.token)
	return b.base.RoundTrip(clone)
}

func bearerClient(base *http.Client, token string) *http.Client {
	return &http.Client{
		Timeout:   base.Timeout,
		Transport: bearerTransport{token: token, base: base.Transport},
	}
}

func connectMCP(ctx context.Context, endpoint string, client *http.Client) (*mcp.ClientSession, error) {
	c := mcp.NewClient(&mcp.Implementation{Name: "sag-e2e", Version: "1"}, nil)
	return c.Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint:             endpoint,
		HTTPClient:           client,
		MaxRetries:           -1,
		DisableStandaloneSSE: true,
	}, nil)
}

func parseToken(output string) (string, error) {
	matches := jwtPattern.FindAllString(output, -1)
	if len(matches) == 0 {
		return "", fmt.Errorf("harness: no JWT in output %q", output)
	}
	return matches[len(matches)-1], nil
}
