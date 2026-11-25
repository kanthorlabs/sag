package endpoint

import (
	"bytes"
	"context"
	"crypto/tls"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kanthorlabs/sag/internal/audit"
	"github.com/kanthorlabs/sag/internal/token"
)

func get(t *testing.T, client *http.Client, url string, headers ...string) (*http.Response, []byte) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range headers {
		req.Header.Add("Authorization", h)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	return resp, body
}

func TestHeaderForm(t *testing.T) {
	rec := &recorder{}
	ts := startServer(t, rec)
	client := ts.client(&tls.Config{})
	raw := validToken(t, "ac03")

	cases := []struct {
		name    string
		headers []string
		want    int
	}{
		{"canonical", []string{"Bearer " + raw}, http.StatusOK},
		{"lower case scheme", []string{"bearer " + raw}, http.StatusOK},
		{"upper case scheme", []string{"BEARER " + raw}, http.StatusOK},
		{"absent", nil, http.StatusUnauthorized},
		{"empty", []string{""}, http.StatusUnauthorized},
		{"scheme only", []string{"Bearer"}, http.StatusUnauthorized},
		{"scheme and space", []string{"Bearer "}, http.StatusUnauthorized},
		{"two spaces", []string{"Bearer  " + raw}, http.StatusUnauthorized},
		{"tab separator", []string{"Bearer\t" + raw}, http.StatusUnauthorized},
		{"comma in token", []string{"Bearer " + raw + ",x"}, http.StatusUnauthorized},
		{"space in token", []string{"Bearer " + raw + " x"}, http.StatusUnauthorized},
		{"basic scheme", []string{"Basic " + raw}, http.StatusUnauthorized},
		{"no scheme", []string{raw}, http.StatusUnauthorized},
		{"two headers", []string{"Bearer " + raw, "Bearer " + raw}, http.StatusUnauthorized},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			before, _, _ := rec.snapshot()
			resp, _ := get(t, client, ts.URL+"/mcp", tc.headers...)
			if resp.StatusCode != tc.want {
				t.Fatalf("status: got %d, want %d", resp.StatusCode, tc.want)
			}
			after, _, _ := rec.snapshot()
			if reached := after > before; reached != (tc.want == http.StatusOK) {
				t.Fatalf("MCP handler reached: %v", reached)
			}
		})
	}
}

func Test401HasWWWAuthenticate(t *testing.T) {
	rec := &recorder{}
	ts := startServer(t, rec)
	client := ts.client(&tls.Config{})
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }

	otherKey := bytes.Repeat([]byte{9}, 32)
	foreign, err := token.Issue(otherKey, "ac03", "Test Agent", time.Now(), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	expired, err := token.Issue(testKey, "ac03", "Test Agent", time.Now().Add(-2*time.Hour), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	future, err := token.Issue(testKey, "ac03", "Test Agent", time.Now().Add(time.Hour), time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name    string
		path    string
		headers []string
		secret  string
	}{
		{"no header", "/mcp", nil, ""},
		{"bad form", "/mcp", []string{"Basic " + foreign}, foreign},
		{"wrong key", "/mcp", []string{"Bearer " + foreign}, foreign},
		{"expired", "/mcp", []string{"Bearer " + expired}, expired},
		{"not yet issued", "/mcp", []string{"Bearer " + future}, future},
		{"garbage", "/mcp", []string{"Bearer not.a.jwt"}, "not.a.jwt"},
		{"other path", "/other", nil, ""},
		{"double slash mcp", "//mcp", nil, ""},
		{"dot segment mcp", "/a/../mcp", nil, ""},
		{"double slash healthz", "//healthz", nil, ""},
		{"healthz trailing slash", "/healthz/", nil, ""},
		{"dot segment healthz", "/a/../healthz", nil, ""},
		{"encoded healthz", "/%68ealthz", nil, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp, body := get(t, client, ts.URL+tc.path, tc.headers...)
			if resp.StatusCode != http.StatusUnauthorized {
				t.Fatalf("status: got %d, want 401", resp.StatusCode)
			}
			if got := resp.Header.Values("WWW-Authenticate"); len(got) != 1 || got[0] != "Bearer" {
				t.Fatalf("WWW-Authenticate: got %q, want [Bearer]", got)
			}
			if tc.secret == "" {
				return
			}
			for name, values := range resp.Header {
				for _, v := range values {
					if strings.Contains(v, tc.secret) {
						t.Fatalf("header %s echoes the token", name)
					}
				}
			}
			if bytes.Contains(body, []byte(tc.secret)) {
				t.Fatal("body echoes the token")
			}
			for _, cause := range []token.Cause{token.CauseExpired, token.CauseNotYetIssued} {
				if bytes.Contains(body, []byte(cause)) {
					t.Fatalf("body discloses cause %q", cause)
				}
			}
		})
	}
	for _, path := range []string{"//mcp", "/a/../mcp", "/healthz"} {
		resp, err := client.Post(ts.URL+path, "application/json", strings.NewReader("{}"))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized || resp.Header.Get("WWW-Authenticate") != "Bearer" {
			t.Fatalf("POST %s: got %d %q, want 401 Bearer", path, resp.StatusCode, resp.Header.Get("WWW-Authenticate"))
		}
	}
	if calls, _, _ := rec.snapshot(); calls != 0 {
		t.Fatalf("MCP handler reached %d times", calls)
	}
}

type bearerTransport struct {
	header string
	base   http.RoundTripper
}

func (b bearerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", b.header)
	return b.base.RoundTrip(r)
}

type whoamiOutput struct {
	Sub        string `json:"sub"`
	JTI        string `json:"jti"`
	RemoteAddr string `json:"remote_addr"`
}

func TestToolSeesSub(t *testing.T) {
	srv := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "test"}, nil)
	mcp.AddTool(srv, &mcp.Tool{Name: "whoami"}, func(ctx context.Context, req *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, whoamiOutput, error) {
		if req.Extra == nil || req.Extra.TokenInfo == nil {
			return nil, whoamiOutput{}, nil
		}
		extra := req.Extra.TokenInfo.Extra
		jti, _ := extra[audit.ExtraJTI].(string)
		addr, _ := extra[audit.ExtraRemoteAddr].(string)
		return nil, whoamiOutput{Sub: req.Extra.TokenInfo.UserID, JTI: jti, RemoteAddr: addr}, nil
	})
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return srv }, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true})
	ts := startServer(t, handler)

	httpClient := ts.client(&tls.Config{})
	httpClient.Transport = bearerTransport{header: "Bearer " + validToken(t, "ac03"), base: httpClient.Transport}
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "test"}, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: ts.URL + "/mcp", HTTPClient: httpClient, DisableStandaloneSSE: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	res, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "whoami", Arguments: map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	out, ok := res.StructuredContent.(map[string]any)
	if !ok || out["sub"] != "ac03" {
		t.Fatalf("structured content: got %#v, want sub ac03", res.StructuredContent)
	}
	if jti, _ := out["jti"].(string); len(jti) != 26 {
		t.Fatalf("jti = %#v; want the token ULID", out["jti"])
	}
	if addr, _ := out["remote_addr"].(string); !strings.HasPrefix(addr, "127.0.0.1:") {
		t.Fatalf("remote_addr = %#v; want the client address and port", out["remote_addr"])
	}
}

type auditCall struct {
	remoteAddr string
	cause      string
}

type auditLog struct {
	mu    sync.Mutex
	calls []auditCall
}

func (l *auditLog) hook(_ context.Context, remoteAddr, cause string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.calls = append(l.calls, auditCall{remoteAddr: remoteAddr, cause: cause})
}

func (l *auditLog) take() []auditCall {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := l.calls
	l.calls = nil
	return out
}

func auditedHandler(t *testing.T, mcp http.Handler, log *auditLog) http.Handler {
	t.Helper()
	cert, _ := testCertificate(t)
	s, err := New(Options{Certificate: cert, JWTKey: testKey, MCP: mcp, Audit: log.hook})
	if err != nil {
		t.Fatal(err)
	}
	return s.Handler()
}

func serve(h http.Handler, method, path string, headers ...string) int {
	req := httptest.NewRequest(method, path, strings.NewReader(`{}`))
	req.RemoteAddr = "192.0.2.7:4100"
	for _, v := range headers {
		req.Header.Add("Authorization", v)
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr.Code
}

func TestAuthFailureAudited(t *testing.T) {
	log := &auditLog{}
	h := auditedHandler(t, &recorder{}, log)
	expired, err := token.Issue(testKey, "ac26", "Test Agent", time.Now().Add(-2*time.Hour), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	future, err := token.Issue(testKey, "ac26", "Test Agent", time.Now().Add(time.Hour), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	valid := validToken(t, "ac26")

	cases := []struct {
		name    string
		path    string
		headers []string
		cause   string
	}{
		{"no header", "/mcp", nil, "missing_bearer"},
		{"basic scheme", "/mcp", []string{"Basic " + valid}, "missing_bearer"},
		{"two headers", "/mcp", []string{"Bearer " + valid, "Bearer " + valid}, "missing_bearer"},
		{"garbage", "/mcp", []string{"Bearer not.a.jwt"}, string(token.CauseUnauthenticated)},
		{"expired", "/mcp", []string{"Bearer " + expired}, string(token.CauseExpired)},
		{"not yet issued", "/mcp", []string{"Bearer " + future}, string(token.CauseNotYetIssued)},
		{"other path", "/other", nil, "missing_bearer"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if code := serve(h, http.MethodPost, tc.path, tc.headers...); code != http.StatusUnauthorized {
				t.Fatalf("status = %d; want 401", code)
			}
			got := log.take()
			if len(got) != 1 || got[0].cause != tc.cause || got[0].remoteAddr != "192.0.2.7:4100" {
				t.Fatalf("audit calls = %+v; want one with cause %q", got, tc.cause)
			}
			for _, h := range tc.headers {
				if strings.Contains(got[0].cause+got[0].remoteAddr, strings.TrimPrefix(h, "Bearer ")) {
					t.Fatal("audit call holds the token")
				}
			}
		})
	}

	if code := serve(h, http.MethodGet, "/healthz"); code != http.StatusOK {
		t.Fatalf("healthz status = %d", code)
	}
	if got := log.take(); len(got) != 0 {
		t.Fatalf("healthz audit calls = %+v; want none", got)
	}

	cert, _ := testCertificate(t)
	s, err := New(Options{Certificate: cert, JWTKey: testKey, MCP: &recorder{}})
	if err != nil {
		t.Fatal(err)
	}
	if code := serve(s.Handler(), http.MethodPost, "/mcp"); code != http.StatusUnauthorized {
		t.Fatalf("status without audit hook = %d; want 401", code)
	}
}

func TestInitializeNotAudited(t *testing.T) {
	log := &auditLog{}
	srv := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "test"}, nil)
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return srv }, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true})
	h := auditedHandler(t, handler, log)
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)

	httpClient := &http.Client{Transport: bearerTransport{header: "Bearer " + validToken(t, "ac26"), base: http.DefaultTransport}}
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "test"}, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: ts.URL + "/mcp", HTTPClient: httpClient, DisableStandaloneSSE: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	if _, err := session.ListTools(ctx, nil); err != nil {
		t.Fatal(err)
	}
	if got := log.take(); len(got) != 0 {
		t.Fatalf("audit calls = %+v; want none for initialize and tools/list", got)
	}
}
