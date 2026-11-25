package endpoint

import (
	"bytes"
	"context"
	"crypto/tls"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

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
	Sub string `json:"sub"`
}

func TestToolSeesSub(t *testing.T) {
	srv := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "test"}, nil)
	mcp.AddTool(srv, &mcp.Tool{Name: "whoami"}, func(ctx context.Context, req *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, whoamiOutput, error) {
		if req.Extra == nil || req.Extra.TokenInfo == nil {
			return nil, whoamiOutput{}, nil
		}
		return nil, whoamiOutput{Sub: req.Extra.TokenInfo.UserID}, nil
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
}
