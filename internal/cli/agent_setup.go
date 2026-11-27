package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/kanthorlabs/sag/internal/agentsetup"
	"github.com/kanthorlabs/sag/internal/hoststate"
	"github.com/kanthorlabs/sag/internal/prompt"
	"github.com/kanthorlabs/sag/internal/token"
)

func agentSetup(args []string, std IO) int {
	if std.Mode == ModeContainer {
		fmt.Fprintln(std.Err, "sag: agent setup runs only on the host")
		return ExitFailure
	}

	opts, ok := parseAgentSetupFlags(args, std)
	if !ok {
		return ExitUsage
	}

	hsPaths, err := hoststate.ResolvePaths()
	if err != nil {
		return fail(std, err)
	}
	opts.CfgDir = hsPaths.Cfg
	opts.AnswersPath = hsPaths.AnswersJSON

	opts.Env = makeAbsoluteConfigDirs(opts.Env)

	answers, err := hoststate.LoadAnswers(hsPaths)
	if err != nil {
		return fail(std, err)
	}

	mode := agentsetup.ModeLocal
	if opts.URL != "" && opts.URL != "https://localhost:8443/mcp" {
		mode, err = agentsetup.ParseURL(opts.URL)
		if err != nil {
			fmt.Fprintf(std.Err, "sag: %v\n", err)
			return ExitUsage
		}
	}

	if mode == agentsetup.ModeLocal {
		podmanPath, err := hoststate.FindPodman(answers.PodmanPath)
		if err != nil {
			return fail(std, err)
		}
		opts.PodmanPath = podmanPath
	}

	agentsetup.SetOut(std.Out)
	if err := agentsetup.Run(context.Background(), opts); err != nil {
		var refusal *prompt.RefusalError
		if errors.As(err, &refusal) {
			fmt.Fprintf(std.Err, "sag: %v\n", err)
			return ExitRefusal
		}
		var conflict *agentsetup.ConflictError
		if errors.As(err, &conflict) {
			return ExitFailure
		}
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

	if harness == agentsetup.HarnessPi && denyRules != "" {
		fmt.Fprintln(std.Err, "sag: --deny-rules is not valid for pi")
		return agentsetup.Options{}, false
	}
	if harness == agentsetup.HarnessClaudeCode && shellProfile != "" {
		fmt.Fprintln(std.Err, "sag: --shell-profile is not valid for claude-code")
		return agentsetup.Options{}, false
	}
	if harness == agentsetup.HarnessClaudeCode && guard != "" {
		fmt.Fprintln(std.Err, "sag: --guard is not valid for claude-code")
		return agentsetup.Options{}, false
	}

	if url != "" && url != "https://localhost:8443/mcp" {
		if _, err := agentsetup.ParseURL(url); err != nil {
			fmt.Fprintf(std.Err, "sag: %v\n", err)
			return agentsetup.Options{}, false
		}
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

func makeAbsoluteConfigDirs(env map[string]string) map[string]string {
	out := make(map[string]string, len(env))
	for k, v := range env {
		out[k] = v
	}
	for _, key := range []string{"CLAUDE_CONFIG_DIR", "PI_CODING_AGENT_DIR"} {
		if v := out[key]; v != "" && !filepath.IsAbs(v) {
			if abs, err := filepath.Abs(v); err == nil {
				out[key] = abs
			}
		}
	}
	return out
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
