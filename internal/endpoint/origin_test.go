package endpoint

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const (
	listedOrigin   = "https://agent.example"
	unlistedOrigin = "https://evil.example"
	originRemote   = "192.0.2.9:4200"
)

type originResult struct {
	code    int
	body    string
	header  http.Header
	reached bool
	audit   []auditCall
}

func originHandler(t *testing.T, allowed []string) (http.Handler, *recorder, *auditLog) {
	t.Helper()
	cert, _ := testCertificate(t)
	rec := &recorder{}
	log := &auditLog{}
	s, err := New(Options{Certificate: cert, JWTKey: testKey, MCP: rec, Audit: log.hook, AllowedOrigins: allowed})
	if err != nil {
		t.Fatal(err)
	}
	return s.Handler(), rec, log
}

func serveOrigin(t *testing.T, allowed []string, method, path, bearer string, origins ...string) originResult {
	t.Helper()
	h, rec, log := originHandler(t, allowed)
	req := httptest.NewRequest(method, path, strings.NewReader(`{}`))
	req.RemoteAddr = originRemote
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	for _, o := range origins {
		req.Header.Add("Origin", o)
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	calls, _, _ := rec.snapshot()
	return originResult{code: rr.Code, body: rr.Body.String(), header: rr.Header(), reached: calls > 0, audit: log.take()}
}

func assertOriginRejected(t *testing.T, got originResult) {
	t.Helper()
	if got.code != http.StatusForbidden {
		t.Fatalf("status = %d; want 403", got.code)
	}
	if got.body != "" {
		t.Fatalf("body = %q; want empty", got.body)
	}
	if v := got.header.Get("WWW-Authenticate"); v != "" {
		t.Fatalf("WWW-Authenticate = %q; want none", v)
	}
	if got.reached {
		t.Fatal("MCP handler reached")
	}
	if len(got.audit) != 1 || got.audit[0].cause != causeOrigin || got.audit[0].remoteAddr != originRemote {
		t.Fatalf("audit calls = %+v; want one with cause %q", got.audit, causeOrigin)
	}
}

func TestOriginOutsideListRejected(t *testing.T) {
	valid := validToken(t, "ac28")
	allowed := []string{listedOrigin, "http://localhost:3000"}
	cases := []struct {
		name    string
		method  string
		path    string
		bearer  string
		origins []string
	}{
		{"unlisted with valid JWT", http.MethodPost, "/mcp", valid, []string{unlistedOrigin}},
		{"unlisted without JWT", http.MethodPost, "/mcp", "", []string{unlistedOrigin}},
		{"unlisted on healthz", http.MethodGet, healthzPath, "", []string{unlistedOrigin}},
		{"trailing slash", http.MethodPost, "/mcp", valid, []string{listedOrigin + "/"}},
		{"upper case", http.MethodPost, "/mcp", valid, []string{"https://Agent.example"}},
		{"other scheme", http.MethodPost, "/mcp", valid, []string{"http://agent.example"}},
		{"other port", http.MethodPost, "/mcp", valid, []string{listedOrigin + ":8443"}},
		{"null", http.MethodPost, "/mcp", valid, []string{"null"}},
		{"empty value", http.MethodPost, "/mcp", valid, []string{""}},
		{"listed pair in one value", http.MethodPost, "/mcp", valid, []string{listedOrigin + ", http://localhost:3000"}},
		{"two listed values", http.MethodPost, "/mcp", valid, []string{listedOrigin, listedOrigin}},
		{"listed and unlisted values", http.MethodPost, "/mcp", valid, []string{listedOrigin, unlistedOrigin}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertOriginRejected(t, serveOrigin(t, allowed, tc.method, tc.path, tc.bearer, tc.origins...))
		})
	}
}

func TestNoOriginPasses(t *testing.T) {
	valid := validToken(t, "ac28")
	for _, allowed := range [][]string{nil, {listedOrigin}} {
		got := serveOrigin(t, allowed, http.MethodPost, "/mcp", valid)
		if got.code != http.StatusOK || !got.reached || len(got.audit) != 0 {
			t.Fatalf("allowed %v: status %d, reached %v, audit %+v; want 200, reached, no audit", allowed, got.code, got.reached, got.audit)
		}
		got = serveOrigin(t, allowed, http.MethodPost, "/mcp", "")
		if got.code != http.StatusUnauthorized || len(got.audit) != 1 || got.audit[0].cause != causeMissingBearer {
			t.Fatalf("allowed %v without JWT: status %d, audit %+v; want 401 with cause %q", allowed, got.code, got.audit, causeMissingBearer)
		}
	}
}

func TestEmptyListRejectsAnyOrigin(t *testing.T) {
	valid := validToken(t, "ac28")
	for _, allowed := range [][]string{nil, {}} {
		for _, origin := range []string{listedOrigin, unlistedOrigin, "http://localhost", "null"} {
			t.Run(origin, func(t *testing.T) {
				assertOriginRejected(t, serveOrigin(t, allowed, http.MethodPost, "/mcp", valid, origin))
			})
		}
	}
}

func TestListedOriginPasses(t *testing.T) {
	valid := validToken(t, "ac28")
	allowed := []string{"http://localhost:3000", listedOrigin}
	for _, origin := range allowed {
		got := serveOrigin(t, allowed, http.MethodPost, "/mcp", valid, origin)
		if got.code != http.StatusOK || !got.reached || len(got.audit) != 0 {
			t.Fatalf("%s: status %d, reached %v, audit %+v; want 200, reached, no audit", origin, got.code, got.reached, got.audit)
		}
	}
	got := serveOrigin(t, allowed, http.MethodPost, "/mcp", "", listedOrigin)
	if got.code != http.StatusUnauthorized || len(got.audit) != 1 || got.audit[0].cause != causeMissingBearer {
		t.Fatalf("listed origin without JWT: status %d, audit %+v; want 401 with cause %q", got.code, got.audit, causeMissingBearer)
	}
	got = serveOrigin(t, allowed, http.MethodGet, healthzPath, "", listedOrigin)
	if got.code != http.StatusOK || len(got.audit) != 0 {
		t.Fatalf("listed origin on healthz: status %d, audit %+v; want 200, no audit", got.code, got.audit)
	}
}
