//go:build e2e

package e2e

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kanthorlabs/sag/e2e/harness"
	"github.com/kanthorlabs/sag/internal/keys"
	"github.com/kanthorlabs/sag/internal/token"
)

const (
	ac26Sub         = "ac26"
	ac26Resource    = "ac26"
	ac26Missing     = "ac26-missing"
	ac26UnknownTool = "ac26_no_such_tool"
	ac26AllowSQL    = "SELECT 1 AS n"
	ac26DenySQL     = "SELECT 1 FOR UPDATE"
	ac26Garbage     = "not.a.jwt"
)

var ac26Fields = []string{"ts", "request_id", "remote_addr", "sub", "jti", "auth_cause", "resource", "capability", "operation", "sql", "sql_sha256", "rendered_sha256", "decision", "reason", "rows", "bytes", "truncated", "duration_ms"}

func init() {
	harness.Register(harness.AC{
		ID:     "AC26",
		Phase:  harness.PhaseSequential,
		Agents: []harness.AgentSpec{{Sub: ac26Sub, Name: "AC26"}},
		Resources: []harness.ResourceSpec{{
			Name:   ac26Resource,
			Agents: []string{ac26Sub},
		}},
		Run: runAC26,
	})
}

type ac26Record map[string]any

func (r ac26Record) str(key string) string {
	s, _ := r[key].(string)
	return s
}

func runAC26(t *testing.T, env harness.Env) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	jwt, err := env.Token(ctx, harness.ContainerSAG, ac26Sub)
	if err != nil {
		t.Fatalf("token: %v", err)
	}
	jwtKey, err := keys.JWTKey(env.MasterKey(), 1)
	if err != nil {
		t.Fatalf("derive JWT key: %v", err)
	}
	expired, err := token.Issue(jwtKey, ac26Sub, "AC26", time.Now().Add(-2*time.Hour), time.Hour)
	if err != nil {
		t.Fatalf("issue expired token: %v", err)
	}

	before := ac26Audit(ctx, t, env)

	session, err := env.Client(ctx, harness.ContainerSAG, ac26Sub)
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	defer session.Close()
	if _, err := session.ListTools(ctx, nil); err != nil {
		t.Fatalf("tools/list: %v", err)
	}
	if n := len(ac26Audit(ctx, t, env)) - len(before); n != 0 {
		t.Fatalf("initialize and tools/list added %d audit records; want 0", n)
	}

	listed, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "list_resources", Arguments: map[string]any{}})
	if err != nil || listed.IsError {
		t.Fatalf("list_resources: %v %+v", err, listed)
	}
	allowID := ac26RequestID(t, ac26Call(ctx, t, session, ac26Resource, ac26AllowSQL), false)
	denyID := ac26RequestID(t, ac26Call(ctx, t, session, ac26Resource, ac26DenySQL), true)
	missingID := ac26RequestID(t, ac26Call(ctx, t, session, ac26Missing, ac26AllowSQL), true)
	if _, err := session.CallTool(ctx, &mcp.CallToolParams{Name: ac26UnknownTool, Arguments: map[string]any{"resource": ac26Resource}}); err == nil {
		t.Fatalf("%s: want an error", ac26UnknownTool)
	}

	ac26Unauthorized(ctx, t, env, "")
	ac26Unauthorized(ctx, t, env, "Bearer "+ac26Garbage)
	ac26Unauthorized(ctx, t, env, "Bearer "+expired)

	after := ac26Audit(ctx, t, env)
	added := after[len(before):]
	if len(added) != 8 {
		t.Fatalf("audit records added = %d; want 8 (5 tools/call, 3 authentication failures): %v", len(added), added)
	}
	for _, raw := range after {
		for _, secret := range []string{jwt, expired, ac26Garbage} {
			if strings.Contains(string(raw), secret) {
				t.Fatalf("audit copy holds a JWT: %s", raw)
			}
		}
	}

	var records []ac26Record
	for _, raw := range added {
		var rec ac26Record
		if err := json.Unmarshal(raw, &rec); err != nil {
			t.Fatalf("audit line %s: %v", raw, err)
		}
		if len(rec) != len(ac26Fields) {
			t.Fatalf("audit line has %d keys; want %d: %s", len(rec), len(ac26Fields), raw)
		}
		for _, f := range ac26Fields {
			if _, ok := rec[f]; !ok {
				t.Fatalf("audit line has no key %q: %s", f, raw)
			}
		}
		if _, err := time.Parse("2006-01-02T15:04:05.000Z", rec.str("ts")); err != nil {
			t.Fatalf("ts %q: %v", rec.str("ts"), err)
		}
		if len(rec.str("request_id")) != 26 || rec.str("remote_addr") == "" {
			t.Fatalf("audit line lacks request_id or remote_addr: %s", raw)
		}
		records = append(records, rec)
	}

	byID := map[string]ac26Record{}
	var auth []ac26Record
	var list, unknown []ac26Record
	for _, rec := range records {
		switch {
		case rec["sub"] == nil:
			auth = append(auth, rec)
		case rec.str("operation") == "list_resources":
			list = append(list, rec)
		case rec.str("operation") == ac26UnknownTool:
			unknown = append(unknown, rec)
		default:
			if _, dup := byID[rec.str("request_id")]; dup {
				t.Fatalf("two audit records for request_id %s", rec.str("request_id"))
			}
			byID[rec.str("request_id")] = rec
		}
	}

	for _, rec := range append(append(list, unknown...), byID[allowID], byID[denyID], byID[missingID]) {
		if rec == nil || rec.str("sub") != ac26Sub || len(rec.str("jti")) != 26 || rec["auth_cause"] != nil {
			t.Fatalf("tools/call record claims = %v", rec)
		}
	}
	if len(list) != 1 || list[0].str("decision") != "allow" || list[0]["rows"] != 1.0 {
		t.Fatalf("list_resources records = %v", list)
	}
	if len(unknown) != 1 || unknown[0].str("decision") != "deny" || unknown[0].str("reason") != "invalid_argument" {
		t.Fatalf("unknown tool records = %v", unknown)
	}

	allow := byID[allowID]
	sum := sha256.Sum256([]byte(ac26AllowSQL))
	if allow.str("decision") != "allow" || allow.str("reason") != "" || allow.str("resource") != ac26Resource || allow.str("capability") != "sql" || allow.str("operation") != "query" ||
		allow.str("sql") != ac26AllowSQL || allow.str("sql_sha256") != hex.EncodeToString(sum[:]) || len(allow.str("rendered_sha256")) != 64 ||
		allow["rows"] != 1.0 || !ac26IsNumber(allow["bytes"]) || allow["truncated"] != false {
		t.Fatalf("allow record = %v", allow)
	}
	deny := byID[denyID]
	if deny.str("decision") != "deny" || !strings.HasPrefix(deny.str("reason"), "query_rejected:") || deny.str("sql") != ac26DenySQL || deny["rows"] != 0.0 {
		t.Fatalf("deny record = %v", deny)
	}
	missing := byID[missingID]
	if missing.str("decision") != "deny" || missing.str("reason") != "not_found" || missing.str("resource") != ac26Missing || missing.str("sql") != "" {
		t.Fatalf("not_found record = %v", missing)
	}

	causes := map[string]bool{}
	for _, rec := range auth {
		if rec["jti"] != nil || rec.str("decision") != "deny" || rec.str("reason") != "unauthenticated" {
			t.Fatalf("authentication failure record = %v", rec)
		}
		causes[rec.str("auth_cause")] = true
	}
	if len(auth) != 3 || !causes["missing_bearer"] || !causes[string(token.CauseUnauthenticated)] || !causes[string(token.CauseExpired)] {
		t.Fatalf("authentication failure records = %v; want missing_bearer, unauthenticated, expired", auth)
	}
}

func ac26Audit(ctx context.Context, t *testing.T, env harness.Env) []json.RawMessage {
	t.Helper()
	records, err := env.Audit(ctx, harness.ContainerSAG)
	if err != nil {
		t.Fatalf("audit copy: %v", err)
	}
	return records
}

func ac26Call(ctx context.Context, t *testing.T, session *mcp.ClientSession, resource, stmt string) *mcp.CallToolResult {
	t.Helper()
	res, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "sql_query", Arguments: map[string]any{"resource": resource, "sql": stmt}})
	if err != nil {
		t.Fatalf("sql_query %q on %s: %v", stmt, resource, err)
	}
	return res
}

func ac26RequestID(t *testing.T, res *mcp.CallToolResult, wantError bool) string {
	t.Helper()
	text := conformanceText(res)
	if res.IsError != wantError {
		t.Fatalf("IsError = %v; want %v: %s", res.IsError, wantError, text)
	}
	var out struct {
		RequestID string `json:"request_id"`
	}
	if err := json.Unmarshal([]byte(text), &out); err != nil || len(out.RequestID) != 26 {
		t.Fatalf("result has no request_id: %s", text)
	}
	return out.RequestID
}

func ac26Unauthorized(ctx context.Context, t *testing.T, env harness.Env, header string) {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimSuffix(env.URL(harness.ContainerSAG), "/")+"/mcp", strings.NewReader(`{}`))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if header != "" {
		req.Header.Set("Authorization", header)
	}
	resp, err := env.HTTPClient().Do(req)
	if err != nil {
		t.Fatalf("POST /mcp: %v", err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("POST /mcp with %q: HTTP %d; want 401", header, resp.StatusCode)
	}
}

func ac26IsNumber(v any) bool {
	_, ok := v.(float64)
	return ok
}
