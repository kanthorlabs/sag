package cli

import (
	"context"
	"flag"
	"fmt"
	"os"
	"runtime"

	"github.com/kanthorlabs/sag/internal/agentsetup"
)

func agentSetup(args []string, std IO) int {
	opts, ok := parseAgentSetupFlags(args, std)
	if !ok {
		return ExitUsage
	}
	agentsetup.SetOut(std.Out)
	if err := agentsetup.Run(context.Background(), opts); err != nil {
		return fail(std, err)
	}
	return ExitOK
}

func parseAgentSetupFlags(args []string, std IO) (agentsetup.Options, bool) {
	fs := flag.NewFlagSet("agent setup", flag.ContinueOnError)
	fs.SetOutput(std.Err)

	var sub string
	fs.StringVar(&sub, "sub", "", "JWT subject")

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

	if fs.NArg() != 1 {
		fmt.Fprintln(std.Err, "usage: sag agent setup <claude-code|pi>")
		return agentsetup.Options{}, false
	}

	harnessName := fs.Arg(0)
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

	env := osEnvMap()

	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(std.Err, "sag: home dir: %v\n", err)
		return agentsetup.Options{}, false
	}

	cfgDir := env["CLAUDE_CONFIG_DIR"]
	if harness == agentsetup.HarnessPi {
		cfgDir = env["PI_CODING_AGENT_DIR"]
	}

	opts := agentsetup.Options{
		Harness:      harness,
		Sub:          sub,
		URL:          url,
		CfgDir:       cfgDir,
		PodmanPath:   "podman",
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
