package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestAgentSetupUnknownHarness(t *testing.T) {
	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}
	std := IO{
		In:         strings.NewReader(""),
		Out:        out,
		Err:        errOut,
		IsTerminal: func(any) bool { return false },
		Mode:       ModeHost,
	}
	code := agentSetup([]string{"unknown-harness"}, std)
	if code != ExitUsage {
		t.Errorf("exit = %d; want ExitUsage (%d)", code, ExitUsage)
	}
}

func TestAgentSetupMissingArg(t *testing.T) {
	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}
	std := IO{
		In:         strings.NewReader(""),
		Out:        out,
		Err:        errOut,
		IsTerminal: func(any) bool { return false },
		Mode:       ModeHost,
	}
	code := agentSetup([]string{}, std)
	if code != ExitUsage {
		t.Errorf("exit = %d; want ExitUsage (%d)", code, ExitUsage)
	}
}

func TestAgentSetupBadDenyRules(t *testing.T) {
	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}
	std := IO{
		In:         strings.NewReader(""),
		Out:        out,
		Err:        errOut,
		IsTerminal: func(any) bool { return false },
		Mode:       ModeHost,
	}
	code := agentSetup([]string{"claude-code", "--deny-rules", "maybe"}, std)
	if code != ExitUsage {
		t.Errorf("exit = %d; want ExitUsage (%d)", code, ExitUsage)
	}
}

func TestParseAgentSetupFlags(t *testing.T) {
	std := IO{
		In:         strings.NewReader(""),
		Out:        &bytes.Buffer{},
		Err:        &bytes.Buffer{},
		IsTerminal: func(any) bool { return false },
		Mode:       ModeHost,
	}
	opts, ok := parseAgentSetupFlags([]string{"claude-code", "--sub", "myagent", "--deny-rules", "yes"}, std)
	if !ok {
		t.Fatal("parseAgentSetupFlags returned false")
	}
	if string(opts.Harness) != "claude-code" {
		t.Errorf("Harness = %q; want claude-code", opts.Harness)
	}
	if opts.Sub != "myagent" {
		t.Errorf("Sub = %q; want myagent", opts.Sub)
	}
	if opts.DenyRules != "yes" {
		t.Errorf("DenyRules = %q; want yes", opts.DenyRules)
	}
}

func TestParseAgentSetupDefaultSub(t *testing.T) {
	std := IO{
		In:         strings.NewReader(""),
		Out:        &bytes.Buffer{},
		Err:        &bytes.Buffer{},
		IsTerminal: func(any) bool { return false },
		Mode:       ModeHost,
	}
	opts, ok := parseAgentSetupFlags([]string{"pi"}, std)
	if !ok {
		t.Fatal("parseAgentSetupFlags returned false")
	}
	if opts.Sub != "pi" {
		t.Errorf("Sub = %q; want pi (default equals harness name)", opts.Sub)
	}
}

func TestParseAgentSetupBadSubExit2(t *testing.T) {
	std := IO{
		In:         strings.NewReader(""),
		Out:        &bytes.Buffer{},
		Err:        &bytes.Buffer{},
		IsTerminal: func(any) bool { return false },
		Mode:       ModeHost,
	}
	_, ok := parseAgentSetupFlags([]string{"claude-code", "--sub", "Bad"}, std)
	if ok {
		t.Error("parseAgentSetupFlags returned true for invalid sub 'Bad'; want false")
	}
}

func TestParseAgentSetupHarnessFirstFlagsAfter(t *testing.T) {
	std := IO{
		In:         strings.NewReader(""),
		Out:        &bytes.Buffer{},
		Err:        &bytes.Buffer{},
		IsTerminal: func(any) bool { return false },
		Mode:       ModeHost,
	}
	opts, ok := parseAgentSetupFlags([]string{"claude-code", "--deny-rules", "no"}, std)
	if !ok {
		t.Fatal("parseAgentSetupFlags returned false for harness-first form")
	}
	if opts.DenyRules != "no" {
		t.Errorf("DenyRules = %q; want no", opts.DenyRules)
	}
}
