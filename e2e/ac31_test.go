//go:build e2e

package e2e

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/kanthorlabs/sag/e2e/harness"
)

const (
	ac31Sub      = "ac31"
	ac31Resource = "ac31"
	ac31Database = "sag_ac31"
	ac31Prompt   = "Use the sag MCP tools. Call list_resources. Then call sql_query on resource ac31 with SELECT marker FROM ac31_marker. Reply with the marker value only."
)

var ac31Marker = ac31NewMarker()

func ac31NewMarker() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return "m31" + hex.EncodeToString(b)
}

func init() {
	harness.Register(harness.AC{
		ID:     "AC31",
		Phase:  harness.PhaseParallel,
		Agents: []harness.AgentSpec{{Sub: ac31Sub, Name: "AC31"}},
		Resources: []harness.ResourceSpec{{
			Name:     ac31Resource,
			Database: ac31Database,
			Agents:   []string{ac31Sub},
		}},
		Fixture: fmt.Sprintf("CREATE TABLE ac31_marker (id int PRIMARY KEY, marker text); INSERT INTO ac31_marker VALUES (1, '%s')", ac31Marker),
		Run:     runAC31,
	})
}

func runAC31(t *testing.T, env harness.Env) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()

	token, err := env.Token(ctx, harness.ContainerSAG, ac31Sub)
	if err != nil {
		t.Fatalf("token: %v", err)
	}
	session, err := env.Client(ctx, harness.ContainerSAG, ac31Sub)
	if err != nil {
		t.Fatalf("client %s: %v", ac31Sub, err)
	}
	ac29Listed(ctx, t, session, ac31Resource)
	session.Close()

	start := time.Now()
	res, err := harness.RunClaude(ctx, harness.ClaudeRequest{
		MCPURL:       env.URL(harness.ContainerSAG) + "/mcp",
		Token:        token,
		CAPEM:        env.CAPEM(),
		Prompt:       ac31Prompt,
		AllowedTools: []string{"mcp__sag__list_resources", "mcp__sag__sql_query"},
	})
	t.Logf("claude ran for %s", time.Since(start).Round(time.Second))
	stdout := strings.ReplaceAll(res.Stdout, token, "<jwt>")
	stderr := strings.ReplaceAll(res.Stderr, token, "<jwt>")
	if err != nil {
		t.Fatalf("claude: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}
	if res.ExitCode != 0 {
		t.Fatalf("claude exit %d\nstdout:\n%s\nstderr:\n%s", res.ExitCode, stdout, stderr)
	}
	if !strings.Contains(res.Stdout, ac31Marker) {
		t.Fatalf("claude output has no marker value\nstdout:\n%s\nstderr:\n%s", stdout, stderr)
	}
	ac31AssertAudit(ctx, t, env)
}

func ac31AssertAudit(ctx context.Context, t *testing.T, env harness.Env) {
	t.Helper()
	records, err := env.Audit(ctx, harness.ContainerSAG)
	if err != nil {
		t.Fatalf("audit copy: %v", err)
	}
	queries := 0
	for _, raw := range records {
		var rec struct {
			Sub       *string `json:"sub"`
			Operation string  `json:"operation"`
			Decision  string  `json:"decision"`
		}
		if err := json.Unmarshal(raw, &rec); err != nil {
			t.Fatalf("audit record: %v", err)
		}
		if rec.Sub == nil || *rec.Sub != ac31Sub {
			continue
		}
		switch rec.Operation {
		case "list_resources":
		case "query":
			if rec.Decision == "allow" {
				queries++
			}
		default:
			t.Fatalf("audit record for %s has operation %q; want only list_resources and query", ac31Sub, rec.Operation)
		}
	}
	if queries != 1 {
		t.Fatalf("allow records with operation query for %s = %d; want 1", ac31Sub, queries)
	}
}
