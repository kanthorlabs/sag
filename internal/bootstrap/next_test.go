package bootstrap

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/kanthorlabs/sag/internal/agentsetup"
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
