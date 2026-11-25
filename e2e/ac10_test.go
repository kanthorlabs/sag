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
	"github.com/kanthorlabs/sag/internal/core"
)

const (
	ac10Sub     = "ac10"
	ac10Strict  = "strict"
	ac10Skip    = "skip"
	ac10Warning = "resource preflight warning"
)

var ac10Results = []string{"resource served", "resource preflight refused", "resource preflight failed"}

func init() {
	harness.Register(harness.AC{
		ID:     "AC10",
		Phase:  harness.PhaseParallel,
		Agents: []harness.AgentSpec{{Sub: ac10Sub, Name: "AC10"}},
		Resources: []harness.ResourceSpec{
			{
				Name:       ac10Strict,
				Containers: []string{harness.ContainerSAG},
				TLS:        core.TLSVerifyFull,
				NoTLSCA:    true,
				Agents:     []string{ac10Sub},
			},
			{
				Name:       ac10Skip,
				Containers: []string{harness.ContainerSAG},
				TLS:        core.TLSInsecureSkipVerify,
				NoTLSCA:    true,
				Agents:     []string{ac10Sub},
			},
		},
		Run: runAC10,
	})
}

type ac10LogLine struct {
	Msg      string `json:"msg"`
	Resource string `json:"resource"`
}

func runAC10(t *testing.T, env harness.Env) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	strict, err := env.Exec(ctx, harness.ContainerSAG, "sag", "resource", "test", ac10Strict)
	if err != nil {
		t.Fatalf("podman exec sag resource test %s: %v", ac10Strict, err)
	}
	if strict.ExitCode == 0 {
		t.Fatalf("sag resource test %s exited 0 with verify-full and an unknown CA; output %q", ac10Strict, strict.Stdout+strict.Stderr)
	}

	skip, err := env.Exec(ctx, harness.ContainerSAG, "sag", "resource", "test", ac10Skip)
	if err != nil {
		t.Fatalf("podman exec sag resource test %s: %v", ac10Skip, err)
	}
	if skip.ExitCode != 0 || strings.Count(skip.Stdout, "warning: ") != 1 {
		t.Fatalf("sag resource test %s exited %d; output %q; want exit 0 with exactly one warning", ac10Skip, skip.ExitCode, skip.Stdout+skip.Stderr)
	}

	names := ac10ListResources(ctx, t, env)
	if !slices.Contains(names, ac10Skip) || slices.Contains(names, ac10Strict) {
		t.Fatalf("list_resources = %v; want %s and no %s", names, ac10Skip, ac10Strict)
	}

	logs, err := env.Logs(ctx, harness.ContainerSAG)
	if err != nil {
		t.Fatalf("podman logs: %v", err)
	}
	skipRuns, skipWarnings, _ := ac10PreflightRuns(logs, ac10Skip)
	if skipRuns < 1 {
		t.Fatalf("daemon log has no preflight result for %s:\n%s", ac10Skip, logs)
	}
	for i, n := range skipWarnings {
		if n != 1 {
			t.Fatalf("preflight run %d of %s logged %d warnings; want exactly one:\n%s", i+1, ac10Skip, n, logs)
		}
	}
	strictRuns, strictWarnings, strictServed := ac10PreflightRuns(logs, ac10Strict)
	if strictRuns < 1 || slices.ContainsFunc(strictWarnings, func(n int) bool { return n != 0 }) {
		t.Fatalf("daemon log for %s has %d runs with warnings %v; want at least one run and no warning:\n%s", ac10Strict, strictRuns, strictWarnings, logs)
	}
	if strictServed {
		t.Fatalf("daemon log serves %s:\n%s", ac10Strict, logs)
	}
}

func ac10PreflightRuns(logs, resource string) (int, []int, bool) {
	var warnings []int
	pending := 0
	served := false
	for _, line := range strings.Split(logs, "\n") {
		var l ac10LogLine
		if json.Unmarshal([]byte(strings.TrimSpace(line)), &l) != nil || l.Resource != resource {
			continue
		}
		switch {
		case l.Msg == ac10Warning:
			pending++
		case slices.Contains(ac10Results, l.Msg):
			served = served || l.Msg == ac10Results[0]
			warnings = append(warnings, pending)
			pending = 0
		}
	}
	return len(warnings), warnings, served
}

func ac10ListResources(ctx context.Context, t *testing.T, env harness.Env) []string {
	t.Helper()
	session, err := env.Client(ctx, harness.ContainerSAG, ac10Sub)
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
