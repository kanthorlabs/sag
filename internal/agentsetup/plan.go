package agentsetup

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/kanthorlabs/sag/internal/hoststate"
	"github.com/kanthorlabs/sag/internal/prompt"
	"github.com/kanthorlabs/sag/internal/secret"
	"github.com/kanthorlabs/sag/internal/stepline"
)

var checkContainerRunning = func(ctx context.Context, podmanPath string) bool {
	out, err := exec.CommandContext(ctx, podmanPath, "container", "inspect", "sag",
		"--format", "{{.State.Running}}").Output()
	return err == nil && strings.TrimSpace(string(out)) == "true"
}

func run(ctx context.Context, opts Options) error {
	out := stepline.New(agentOut)

	if opts.GOOS == "" {
		opts.GOOS = "darwin"
	}

	mode := ModeLocal
	if opts.URL != "" && opts.URL != "https://localhost:8443/mcp" {
		var err error
		mode, err = ParseURL(opts.URL)
		if err != nil {
			return err
		}
	}

	if flag, needed := needsInteractiveAnswer(opts); needed {
		tty, err := remoteOpenTTY()
		if err != nil {
			return fmt.Errorf("add %s flag or run from a terminal: %w", flag, prompt.ErrNoTerminal)
		}
		tty.Close()
	}

	containerRuns := false
	if mode == ModeLocal {
		containerRuns = checkContainerRunning(ctx, opts.PodmanPath)
	}

	conflicts := planConflicts(opts, mode, containerRuns)
	for _, c := range conflicts {
		out.Line(stepline.StatusFail, c, "")
	}
	if len(conflicts) > 0 {
		return fmt.Errorf("plan conflicts: %s", strings.Join(conflicts, "; "))
	}

	var lock *hoststate.Lock
	if !opts.UnderLock {
		p, err := hoststate.ResolvePaths()
		if err != nil {
			return err
		}
		lock, err = hoststate.AcquireLock(p)
		if err != nil {
			return err
		}
		defer lock.Release()

		if mode == ModeLocal {
			containerRuns = checkContainerRunning(ctx, opts.PodmanPath)
		}
		conflicts = planConflicts(opts, mode, containerRuns)
		for _, c := range conflicts {
			out.Line(stepline.StatusFail, c, "")
		}
		if len(conflicts) > 0 {
			return fmt.Errorf("plan conflicts after lock: %s", strings.Join(conflicts, "; "))
		}
	}

	var jwtCred secret.Credential
	switch mode {
	case ModeRemote:
		cred, err := remoteJWT(ctx, opts)
		if err != nil {
			return err
		}
		jwtCred = cred
	default:
		cred, err := issueJWT(ctx, opts)
		if err != nil {
			return err
		}
		jwtCred = cred
	}

	if err := registerMCP(ctx, opts, jwtCred); err != nil {
		return err
	}

	if err := installSkill(ctx, opts); err != nil {
		return err
	}

	if err := applyCATrust(ctx, opts); err != nil {
		return err
	}

	if err := applyDenyRules(ctx, opts, harnessPathsFromOpts(opts).Dir); err != nil {
		return err
	}

	if err := applyShellProfile(ctx, opts); err != nil {
		return err
	}

	if err := applyGuard(ctx, opts, harnessPathsFromOpts(opts).Dir, sagGuardTS); err != nil {
		return err
	}

	if err := verify(ctx, opts); err != nil {
		return err
	}

	harnessName := string(opts.Harness)
	suffix := ""
	if opts.Harness == HarnessPi {
		suffix = " from a new shell"
	}
	out.Line("ok", "agent "+harnessName+"; restart "+harnessName+suffix, "")
	return nil
}

func planOnly(ctx context.Context, opts Options) (Plan, error) {
	if opts.GOOS == "" {
		opts.GOOS = "darwin"
	}
	mode := ModeLocal
	if opts.URL != "" && opts.URL != "https://localhost:8443/mcp" {
		var err error
		mode, err = ParseURL(opts.URL)
		if err != nil {
			return Plan{}, err
		}
	}
	containerRuns := false
	if mode == ModeLocal {
		containerRuns = checkContainerRunning(ctx, opts.PodmanPath)
	}
	conflicts := planConflicts(opts, mode, containerRuns)
	return Plan{Conflicts: conflicts}, nil
}

func needsInteractiveAnswer(opts Options) (flag string, needed bool) {
	switch opts.Harness {
	case HarnessClaudeCode:
		if opts.DenyRules != "" {
			return "", false
		}
		paths := harnessPathsFromOpts(opts)
		current, _ := readDenyRules(paths.SettingsFile)
		if len(appendMissingRules(current, sagDenyRules)) > len(current) {
			return "--deny-rules", true
		}
	case HarnessPi:
		if opts.ShellProfile != "" {
			return "", false
		}
		shell := opts.Env["SHELL"]
		zdotdir := opts.Env["ZDOTDIR"]
		profile, ok := ShellProfile(opts.GOOS, shell, zdotdir, opts.Home)
		if !ok {
			return "--shell-profile", true
		}
		if !fileContainsLine(profile, `export NODE_EXTRA_CA_CERTS="`+caFilePath(opts)+`"`) {
			return "--shell-profile", true
		}
	}
	return "", false
}

func planConflicts(opts Options, mode Mode, containerRuns bool) []string {
	var conflicts []string

	paths := harnessPathsFromOpts(opts)

	if !harnessInPATH(opts) {
		conflicts = append(conflicts, "harness "+string(opts.Harness)+" not in PATH")
	}

	if mode == ModeLocal && !containerRuns && !opts.UnderLock {
		conflicts = append(conflicts, "container sag does not run")
	}

	if mode == ModeRemote {
		if _, err := os.Stat(caFilePath(opts)); err != nil {
			conflicts = append(conflicts, "ca.pem absent for remote mode")
		}
	}

	if c := checkMCPFile(paths.MCPFile); c != "" {
		conflicts = append(conflicts, c)
	}

	if paths.SettingsFile != "" {
		if c := checkJSONFile(paths.SettingsFile); c != "" {
			conflicts = append(conflicts, c)
		}
	}

	if paths.SkillFile != "" {
		if c := checkSkillFile(paths.SkillFile); c != "" {
			conflicts = append(conflicts, c)
		}
	}

	if paths.GuardFile != "" {
		if c := checkGuardFile(paths.GuardFile); c != "" {
			conflicts = append(conflicts, c)
		}
	}

	if c := checkCATrustConflict(opts, paths); c != "" {
		conflicts = append(conflicts, c)
	}

	if opts.Harness == HarnessPi && opts.ShellProfile == "yes" {
		shell := opts.Env["SHELL"]
		_, ok := ShellProfile(opts.GOOS, shell, opts.Env["ZDOTDIR"], opts.Home)
		if !ok {
			conflicts = append(conflicts, "shell-profile: unsupported shell "+shell+"; add export NODE_EXTRA_CA_CERTS manually then run with --shell-profile no")
		}
	}

	return conflicts
}

func harnessInPATH(opts Options) bool {
	name := string(opts.Harness)
	if name == string(HarnessClaudeCode) {
		name = "claude"
	}
	_, err := lookupHarness(opts, name)
	return err == nil
}

func checkMCPFile(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(data, &top); err != nil {
		return "mcp-config " + path + " is not valid JSON"
	}
	mcpRaw, ok := top["mcpServers"]
	if !ok {
		return ""
	}
	var servers map[string]json.RawMessage
	if err := json.Unmarshal(mcpRaw, &servers); err != nil {
		return "mcpServers in " + path + " is not valid JSON"
	}
	sagRaw, ok := servers["sag"]
	if !ok {
		return ""
	}
	var entry map[string]json.RawMessage
	if err := json.Unmarshal(sagRaw, &entry); err != nil {
		return "mcpServers.sag in " + path + " is not valid JSON"
	}
	if _, hasCmd := entry["command"]; hasCmd {
		return "mcp-server sag in " + path + " has another form; remove it"
	}
	urlRaw, ok := entry["url"]
	if !ok {
		return ""
	}
	var entryURL string
	if err := json.Unmarshal(urlRaw, &entryURL); err != nil {
		return "mcp-server sag in " + path + " has another form; remove it"
	}
	headersRaw, ok := entry["headers"]
	if !ok {
		return ""
	}
	var headers map[string]string
	if err := json.Unmarshal(headersRaw, &headers); err != nil {
		return "mcp-server sag in " + path + " has another form; remove it"
	}
	auth := headers["Authorization"]
	if auth != "" && !strings.HasPrefix(auth, "Bearer ") {
		return "mcp-server sag in " + path + " has another form; remove it"
	}
	return ""
}

func checkJSONFile(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return "settings " + path + " is not valid JSON"
	}
	return ""
}

func checkSkillFile(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	if !strings.Contains(string(data), skillMarker) {
		return "skill " + path + " exists without the managed marker"
	}
	return ""
}

func checkGuardFile(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	lines := strings.SplitN(string(data), "\n", 2)
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != guardMarker {
		return "guard " + path + " exists without the managed marker in line 1"
	}
	return ""
}

func checkCATrustConflict(opts Options, paths HarnessPaths) string {
	caPath := caFilePath(opts)
	switch opts.Harness {
	case HarnessPi:
		shell := opts.Env["SHELL"]
		zdotdir := opts.Env["ZDOTDIR"]
		profile, ok := ShellProfile(opts.GOOS, shell, zdotdir, opts.Home)
		if !ok {
			return ""
		}
		current := currentNECCPi(opts, profile)
		if current != "" && !acceptsCATrust(current, caPath) {
			bundlePath := caPath[:len(caPath)-len(".pem")] + "-bundle.pem"
			return fmt.Sprintf("ca-trust NODE_EXTRA_CA_CERTS is %s; run: cat %q %q > %q; then set NODE_EXTRA_CA_CERTS to %s in %s",
				current, current, caPath, bundlePath, bundlePath, profile)
		}
	default:
		if paths.SettingsFile == "" {
			return ""
		}
		current := currentNECCClaudeCode(opts, paths.SettingsFile)
		if current != "" && !acceptsCATrust(current, caPath) {
			bundlePath := caPath[:len(caPath)-len(".pem")] + "-bundle.pem"
			src := opts.Env["NODE_EXTRA_CA_CERTS"]
			if src == "" {
				src = paths.SettingsFile
			}
			return fmt.Sprintf("ca-trust NODE_EXTRA_CA_CERTS is %s; run: cat %q %q > %q; then set NODE_EXTRA_CA_CERTS to %s in %s",
				current, current, caPath, bundlePath, bundlePath, src)
		}
	}
	return ""
}
