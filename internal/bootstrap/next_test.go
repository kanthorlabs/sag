package bootstrap

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/kanthorlabs/sag/internal/agentsetup"
	"github.com/kanthorlabs/sag/internal/prompt"
	"github.com/kanthorlabs/sag/internal/stepline"
	"github.com/kanthorlabs/sag/internal/tailscale"
)

func TestNextCommandAgents(t *testing.T) {
	dir := t.TempDir()
	var out bytes.Buffer

	d := agentTestDeps(dir)
	d.Print = &out

	as := &agentRunState{
		detected: []harnessDet{
			{harness: agentsetup.HarnessClaudeCode, sub: "claude-code"},
			{harness: agentsetup.HarnessPi, sub: "pi"},
		},
		completed: []harnessDet{
			{harness: agentsetup.HarnessClaudeCode, sub: "claude-code"},
			{harness: agentsetup.HarnessPi, sub: "pi"},
		},
	}
	s := &State{Out: stepline.New(io.Discard)}

	if err := d.printEndpoint(context.Background(), s, as); err != nil {
		t.Fatalf("printEndpoint: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "--agent claude-code") {
		t.Fatalf("output missing --agent claude-code; got:\n%s", output)
	}
	if !strings.Contains(output, "--agent pi") {
		t.Fatalf("output missing --agent pi; got:\n%s", output)
	}
	if !strings.Contains(output, "https://localhost:8443/mcp") {
		t.Fatalf("output missing endpoint; got:\n%s", output)
	}
}

func TestNextCommandOneAgent(t *testing.T) {
	dir := t.TempDir()
	var out bytes.Buffer

	d := agentTestDeps(dir)
	d.Print = &out

	as := &agentRunState{
		detected: []harnessDet{
			{harness: agentsetup.HarnessClaudeCode, sub: "claude-code"},
			{harness: agentsetup.HarnessPi, sub: "pi"},
		},
		completed: []harnessDet{
			{harness: agentsetup.HarnessClaudeCode, sub: "claude-code"},
		},
	}
	s := &State{Out: stepline.New(io.Discard)}

	if err := d.printEndpoint(context.Background(), s, as); err != nil {
		t.Fatalf("printEndpoint: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "--agent claude-code") {
		t.Fatalf("output missing --agent claude-code; got:\n%s", output)
	}
	if strings.Contains(output, "--agent pi") {
		t.Fatalf("output has --agent pi but pi was not completed; got:\n%s", output)
	}
	if !strings.Contains(output, "sag agent setup pi") {
		t.Fatalf("output missing 'sag agent setup pi'; got:\n%s", output)
	}
}

func TestNoCompletedHarness(t *testing.T) {
	dir := t.TempDir()
	var out bytes.Buffer

	d := agentTestDeps(dir)
	d.Print = &out

	as := &agentRunState{
		detected:  []harnessDet{{harness: agentsetup.HarnessClaudeCode, sub: "claude-code"}},
		completed: nil,
	}
	s := &State{Out: stepline.New(io.Discard)}

	if err := d.printEndpoint(context.Background(), s, as); err != nil {
		t.Fatalf("printEndpoint: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "--agent <sub>") {
		t.Fatalf("output missing --agent <sub> placeholder; got:\n%s", output)
	}
	if strings.Contains(output, "--agent claude-code") {
		t.Fatalf("output has --agent claude-code but no completed harness; got:\n%s", output)
	}
	if !strings.Contains(output, "sag agent setup claude-code") {
		t.Fatalf("output missing 'sag agent setup claude-code'; got:\n%s", output)
	}
}

func TestNextCommandNoDetectedHarness(t *testing.T) {
	dir := t.TempDir()
	var out bytes.Buffer

	d := agentTestDeps(dir)
	d.Print = &out

	as := &agentRunState{}
	s := &State{Out: stepline.New(io.Discard)}

	if err := d.printEndpoint(context.Background(), s, as); err != nil {
		t.Fatalf("printEndpoint: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "--agent <sub>") {
		t.Fatalf("output missing --agent <sub> placeholder; got:\n%s", output)
	}
}

func TestNextCommandTailscaleSteps(t *testing.T) {
	dir := t.TempDir()
	var out bytes.Buffer

	d := agentTestDeps(dir)
	d.Print = &out

	as := &agentRunState{
		completed: []harnessDet{{harness: agentsetup.HarnessClaudeCode, sub: "claude-code"}},
	}
	s := &State{
		Out:            stepline.New(io.Discard),
		TailscaleState: tailscale.State{Address: "100.64.0.1"},
	}

	if err := d.printEndpoint(context.Background(), s, as); err != nil {
		t.Fatalf("printEndpoint: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "100.64.0.1") {
		t.Fatalf("output missing tailscale address; got:\n%s", output)
	}
}

func TestTailscaleStepsNotPrintedWhenDown(t *testing.T) {
	dir := t.TempDir()
	var out bytes.Buffer

	d := agentTestDeps(dir)
	d.Print = &out

	as := &agentRunState{}
	s := &State{
		Out: stepline.New(io.Discard),
		TailscaleState: tailscale.State{
			Address:    "100.64.0.1",
			SkipReason: tailscale.SkipDown,
		},
	}

	if err := d.printEndpoint(context.Background(), s, as); err != nil {
		t.Fatalf("printEndpoint: %v", err)
	}

	output := out.String()
	if strings.Contains(output, "100.64.0.1") {
		t.Fatalf("tailscale steps printed when Tailscale is down; got:\n%s", output)
	}
}

func TestCATrustConflictStep15Runs(t *testing.T) {
	dir := t.TempDir()
	claudeDir := t.TempDir()
	var out bytes.Buffer

	d := agentTestDeps(dir)
	d.Print = &out
	d.HarnessInPATH = func(h agentsetup.Harness, environ map[string]string) bool {
		return h == agentsetup.HarnessClaudeCode
	}
	d.Environ = func() map[string]string {
		return map[string]string{"CLAUDE_CONFIG_DIR": claudeDir}
	}
	d.AgentRun = func(ctx context.Context, opts agentsetup.Options) error {
		return errCATrustConflict
	}
	d.ResolveYesNo = func(a prompt.Asker, flagValue, stored, question, flagName string) (string, error) {
		if stored != "" {
			return stored, nil
		}
		if flagName == "--agent-setup" {
			return "yes", nil
		}
		if flagValue != "" {
			return flagValue, nil
		}
		return "yes", nil
	}

	err := Run(context.Background(), Flags{Autostart: "yes"}, d)
	if err == nil {
		t.Fatal("Run: want error (ca-trust conflict), got nil")
	}
	if !errors.Is(err, errCATrustConflict) {
		t.Fatalf("error = %v; want errCATrustConflict", err)
	}
	output := out.String()
	if !strings.Contains(output, "https://localhost:8443/mcp") {
		t.Fatalf("step 15 did not run; endpoint not printed; output = %q", output)
	}
}
