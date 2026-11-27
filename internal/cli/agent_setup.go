package cli

import (
	"context"
	"flag"
	"fmt"
	"os"
	"runtime"

	"github.com/kanthorlabs/sag/internal/agentsetup"
	"github.com/kanthorlabs/sag/internal/hoststate"
	"github.com/kanthorlabs/sag/internal/token"
)

func agentSetup(args []string, std IO) int {
	opts, ok := parseAgentSetupFlags(args, std)
	if !ok {
		return ExitUsage
	}

	hsPaths, err := hoststate.ResolvePaths()
	if err != nil {
		return fail(std, err)
	}
	opts.CfgDir = hsPaths.Cfg

	answers, err := hoststate.LoadAnswers(hsPaths)
	if err != nil {
		return fail(std, err)
	}
	podmanPath, err := hoststate.FindPodman(answers.PodmanPath)
	if err != nil {
		return fail(std, err)
	}
	opts.PodmanPath = podmanPath

	agentsetup.SetOut(std.Out)
	if err := agentsetup.Run(context.Background(), opts); err != nil {
		return fail(std, err)
	}
	return ExitOK
}

func parseAgentSetupFlags(args []string, std IO) (agentsetup.Options, bool) {
	if len(args) == 0 {
		fmt.Fprintln(std.Err, "usage: sag agent setup <claude-code|pi>")
		return agentsetup.Options{}, false
	}
	harnessName := args[0]
	args = args[1:]

	var harness agentsetup.Harness
	switch harnessName {
	case "claude-code":
		harness = agentsetup.HarnessClaudeCode
	case "pi":
		harness = agentsetup.HarnessPi
	default:
		fmt.Fprintf(std.Err, "sag: agent setup: unknown harness %q; want claude-code or pi\n", harnessName)
		return agentsetup.Options{}, false
	}

	fs := flag.NewFlagSet("agent setup", flag.ContinueOnError)
	fs.SetOutput(std.Err)

	var sub string
	fs.StringVar(&sub, "sub", "", "JWT subject (default: harness name)")

	var url string
	fs.StringVar(&url, "url", "https://localhost:8443/mcp", "MCP server URL")

	var denyRules string
	fs.StringVar(&denyRules, "deny-rules", "", "yes|no")

	var shellProfile string
	fs.StringVar(&shellProfile, "shell-profile", "", "yes|no")

	var guard string
	fs.StringVar(&guard, "guard", "", "yes|no")

	if err := fs.Parse(args); err != nil {
		return agentsetup.Options{}, false
	}

	if fs.NArg() != 0 {
		fmt.Fprintf(std.Err, "sag: agent setup: unexpected arguments: %v\n", fs.Args())
		return agentsetup.Options{}, false
	}

	if !validYesNoFlag("--deny-rules", denyRules, std.Err) {
		return agentsetup.Options{}, false
	}
	if !validYesNoFlag("--shell-profile", shellProfile, std.Err) {
		return agentsetup.Options{}, false
	}
	if !validYesNoFlag("--guard", guard, std.Err) {
		return agentsetup.Options{}, false
	}

	if sub == "" {
		sub = harnessName
	}
	if !token.ValidSub(sub) {
		fmt.Fprintf(std.Err, "sag: --sub: invalid subject %q; must match ^[a-z][a-z0-9-]{0,62}$\n", sub)
		return agentsetup.Options{}, false
	}

	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(std.Err, "sag: home dir: %v\n", err)
		return agentsetup.Options{}, false
	}

	env := osEnvMap()

	opts := agentsetup.Options{
		Harness:      harness,
		Sub:          sub,
		URL:          url,
		Env:          env,
		Home:         home,
		GOOS:         runtime.GOOS,
		DenyRules:    denyRules,
		ShellProfile: shellProfile,
		Guard:        guard,
	}
	return opts, true
}

func osEnvMap() map[string]string {
	m := make(map[string]string)
	for _, kv := range os.Environ() {
		for i, c := range kv {
			if c == '=' {
				m[kv[:i]] = kv[i+1:]
				break
			}
		}
	}
	return m
}
