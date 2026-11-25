//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kanthorlabs/sag/e2e/harness"
)

const (
	ac11Sub           = "ac11"
	ac11Resource      = "ext"
	ac11Database      = "sag_ext"
	ac11RecordPath    = "/var/lib/sag/approvals/ext.json"
	ac11ServeWindow   = 70 * time.Second
	ac11RecheckWindow = 20 * time.Second
	ac11Hint          = "podman exec -it sag sag extension approve ext"
	ac11InvalidRecord = "approval record is not valid for the current objects"
)

func init() {
	harness.Register(harness.AC{
		ID:     "AC11",
		Phase:  harness.PhaseParallel,
		Agents: []harness.AgentSpec{{Sub: ac11Sub, Name: "AC11"}},
		Resources: []harness.ResourceSpec{{
			Name:       ac11Resource,
			Containers: []string{harness.ContainerSAG},
			Database:   ac11Database,
			Agents:     []string{ac11Sub},
		}},
		Fixture: "CREATE EXTENSION IF NOT EXISTS dblink",
		Run:     runAC11,
	})
}

func runAC11(t *testing.T, env harness.Env) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	sag := env.ContainerName(harness.ContainerSAG)

	t.Run("refused before approval", func(t *testing.T) {
		res := ac11ResourceTest(ctx, t, env)
		if res.ExitCode == 0 {
			t.Fatalf("sag resource test %s exited 0 with dblink and no approval; output %q", ac11Resource, res.Stdout+res.Stderr)
		}
		if !strings.Contains(res.Stdout+res.Stderr, ac11Hint) {
			t.Fatalf("sag resource test %s output %q; want the hint %q", ac11Resource, res.Stdout+res.Stderr, ac11Hint)
		}
	})

	t.Run("approve on a terminal", func(t *testing.T) {
		tty, err := env.ExecTTY(ctx, harness.ContainerSAG, "sag", "extension", "approve", ac11Resource)
		if err != nil {
			t.Fatalf("podman exec -it: %v", err)
		}
		if err := tty.Expect(ctx, "type the resource name "+ac11Resource+" to approve"); err != nil {
			t.Fatal(err)
		}
		if err := tty.Send(ac11Resource + "\r"); err != nil {
			t.Fatalf("send confirmation: %v", err)
		}
		res, err := tty.Wait(ctx)
		if err != nil {
			t.Fatalf("wait: %v", err)
		}
		if res.ExitCode != 0 {
			t.Fatalf("sag extension approve %s exited %d; output %q", ac11Resource, res.ExitCode, res.Stdout)
		}
		if !strings.Contains(res.Stdout, "dblink") || !strings.Contains(res.Stdout, "approval written") {
			t.Fatalf("sag extension approve %s output %q; want dblink and the write message", ac11Resource, res.Stdout)
		}
	})

	t.Run("served after approval", func(t *testing.T) {
		if res := ac11ResourceTest(ctx, t, env); res.ExitCode != 0 {
			t.Fatalf("sag resource test %s after approval exited %d; output %q", ac11Resource, res.ExitCode, res.Stdout+res.Stderr)
		}
		deadline := time.Now().Add(ac11ServeWindow)
		for {
			names := ac11ListResources(ctx, t, env)
			if slices.Contains(names, ac11Resource) {
				return
			}
			if time.Now().After(deadline) {
				t.Fatalf("list_resources = %v after %v; want %s", names, ac11ServeWindow, ac11Resource)
			}
			time.Sleep(2 * time.Second)
		}
	})

	t.Run("changed mac refused", func(t *testing.T) {
		original, err := harness.ReadContainerFile(ctx, sag, ac11RecordPath)
		if err != nil {
			t.Fatalf("read approval record: %v", err)
		}
		tampered, err := ac11FlipMACByte(original)
		if err != nil {
			t.Fatal(err)
		}
		if err := harness.WriteContainerFile(ctx, sag, ac11RecordPath, tampered, 0o600); err != nil {
			t.Fatalf("write tampered record on the read-only container: %v", err)
		}
		res := ac11ResourceTest(ctx, t, env)
		if err := harness.WriteContainerFile(ctx, sag, ac11RecordPath, original, 0o600); err != nil {
			t.Fatalf("restore approval record: %v", err)
		}
		if res.ExitCode == 0 {
			t.Fatalf("sag resource test %s accepted a record with a changed mac; output %q", ac11Resource, res.Stdout+res.Stderr)
		}
		if !strings.Contains(res.Stdout, ac11InvalidRecord) {
			t.Fatalf("sag resource test %s output %q after a changed mac; want %q", ac11Resource, res.Stdout+res.Stderr, ac11InvalidRecord)
		}
		if res := ac11ResourceTest(ctx, t, env); res.ExitCode != 0 {
			t.Fatalf("sag resource test %s after restore exited %d; output %q", ac11Resource, res.ExitCode, res.Stdout+res.Stderr)
		}
		deadline := time.Now().Add(ac11ServeWindow)
		for ac11QueryCode(ctx, t, env) == "resource_unavailable" {
			if time.Now().After(deadline) {
				t.Fatalf("sql_query on %s returned resource_unavailable %v after the record restore", ac11Resource, ac11ServeWindow)
			}
			time.Sleep(2 * time.Second)
		}
	})

	t.Run("new risky extension removes the resource", func(t *testing.T) {
		conn, err := harness.ConnectDB(ctx, env.Postgres(env.Major()), ac11Database)
		if err != nil {
			t.Fatalf("connect %s: %v", ac11Database, err)
		}
		_, err = conn.PgConn().ExecParams(ctx, "CREATE EXTENSION IF NOT EXISTS file_fdw", nil, nil, nil, nil).Close()
		conn.Close(ctx)
		if err != nil {
			t.Fatalf("CREATE EXTENSION file_fdw: %v", err)
		}
		deadline := time.Now().Add(ac11RecheckWindow)
		for {
			code := ac11QueryCode(ctx, t, env)
			if code == "resource_unavailable" {
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("sql_query on %s returned %q %v after CREATE EXTENSION file_fdw; want resource_unavailable", ac11Resource, code, ac11RecheckWindow)
			}
			time.Sleep(time.Second)
		}
		res := ac11ResourceTest(ctx, t, env)
		if res.ExitCode == 0 || !strings.Contains(res.Stdout+res.Stderr, "file_fdw") {
			t.Fatalf("sag resource test %s after file_fdw exited %d; output %q; want a refusal that names file_fdw", ac11Resource, res.ExitCode, res.Stdout+res.Stderr)
		}
	})
}

type ac11Result struct {
	ExitCode int
	Stdout   string
	Stderr   string
}

func ac11ResourceTest(ctx context.Context, t *testing.T, env harness.Env) ac11Result {
	t.Helper()
	res, err := env.Exec(ctx, harness.ContainerSAG, "sag", "resource", "test", ac11Resource)
	if err != nil {
		t.Fatalf("podman exec sag resource test %s: %v", ac11Resource, err)
	}
	return ac11Result{ExitCode: res.ExitCode, Stdout: res.Stdout, Stderr: res.Stderr}
}

func ac11FlipMACByte(record []byte) ([]byte, error) {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(record, &m); err != nil {
		return nil, err
	}
	var mac string
	if err := json.Unmarshal(m["mac"], &mac); err != nil {
		return nil, err
	}
	b := []byte(mac)
	if b[0] == '0' {
		b[0] = '1'
	} else {
		b[0] = '0'
	}
	raw, err := json.Marshal(string(b))
	if err != nil {
		return nil, err
	}
	m["mac"] = raw
	return json.Marshal(m)
}

func ac11ListResources(ctx context.Context, t *testing.T, env harness.Env) []string {
	t.Helper()
	session, err := env.Client(ctx, harness.ContainerSAG, ac11Sub)
	if err != nil {
		t.Fatalf("MCP client: %v", err)
	}
	defer session.Close()
	res, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "list_resources"})
	if err != nil {
		t.Fatalf("list_resources: %v", err)
	}
	if res.IsError {
		t.Fatalf("list_resources returned an error result: %+v", res.Content)
	}
	raw, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var out struct {
		Resources []struct {
			Name string `json:"name"`
		} `json:"resources"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("list_resources content %s: %v", raw, err)
	}
	var names []string
	for _, r := range out.Resources {
		names = append(names, r.Name)
	}
	return names
}

func ac11QueryCode(ctx context.Context, t *testing.T, env harness.Env) string {
	t.Helper()
	session, err := env.Client(ctx, harness.ContainerSAG, ac11Sub)
	if err != nil {
		t.Fatalf("MCP client: %v", err)
	}
	defer session.Close()
	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "sql_query",
		Arguments: map[string]any{"resource": ac11Resource, "sql": "SELECT 1"},
	})
	if err != nil {
		t.Fatalf("sql_query: %v", err)
	}
	if !res.IsError {
		return ""
	}
	for _, c := range res.Content {
		text, ok := c.(*mcp.TextContent)
		if !ok {
			continue
		}
		var body struct {
			Code string `json:"code"`
		}
		if json.Unmarshal([]byte(text.Text), &body) == nil {
			return body.Code
		}
	}
	return ""
}
