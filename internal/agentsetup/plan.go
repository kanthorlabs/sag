package agentsetup

import (
	"context"
	"encoding/json"
	"errors"
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

var newAsker = prompt.NewAsker

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

	asker := opts.Asker
	if asker.Open == nil {
		asker = newAsker()
	}

	answers, err := resolveAnswers(opts, asker)
	if err != nil {
		return err
	}
	applyAnswers(&opts, answers)

	containerRuns := false
	if mode == ModeLocal {
		containerRuns = checkContainerRunning(ctx, opts.PodmanPath)
	}

	conflicts, conflictErr := planConflictsWithErr(opts, mode, containerRuns)
	for _, c := range conflicts {
		out.Line(stepline.StatusFail, c, "")
	}
	if conflictErr != nil {
		return conflictErr
	}

	var keptJWT string
	if mode == ModeRemote {
		keptJWT = checkKeptRemoteEntry(opts)
		if keptJWT == "" {
			if err := checkRemoteTerminal(); err != nil {
				return err
			}
		}
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
		conflicts, conflictErr = planConflictsWithErr(opts, mode, containerRuns)
		for _, c := range conflicts {
			out.Line(stepline.StatusFail, c, "")
		}
		if conflictErr != nil {
			return conflictErr
		}
	}

	answers.save()

	var jwtFunc func() (secret.Credential, error)
	switch mode {
	case ModeRemote:
		captured := keptJWT
		if captured != "" {
			jwtFunc = func() (secret.Credential, error) {
				return secret.NewCredential("jwt-"+opts.Sub, []byte(captured)), nil
			}
		} else {
			jwtFunc = func() (secret.Credential, error) { return remoteJWT(ctx, opts) }
		}
	default:
		jwtFunc = func() (secret.Credential, error) { return issueJWT(ctx, opts) }
	}

	if err := registerMCP(ctx, opts, jwtFunc); err != nil {
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

	asker := opts.Asker
	if asker.Open == nil {
		asker = newAsker()
	}

	answers, err := resolveAnswers(opts, asker)
	if err != nil {
		return Plan{}, err
	}
	applyAnswers(&opts, answers)

	containerRuns := false
	if mode == ModeLocal {
		containerRuns = checkContainerRunning(ctx, opts.PodmanPath)
	}

	conflicts, conflictErr := planConflictsWithErr(opts, mode, containerRuns)
	out := stepline.New(agentOut)
	for _, c := range conflicts {
		out.Line(stepline.StatusFail, c, "")
	}
	if conflictErr != nil {
		return Plan{}, conflictErr
	}

	if mode == ModeRemote {
		keptJWT := checkKeptRemoteEntry(opts)
		if keptJWT == "" {
			if err := checkRemoteTerminal(); err != nil {
				return Plan{}, err
			}
		}
	}

	return Plan{Answers: answers, Conflicts: conflicts}, nil
}

func resolveAnswers(opts Options, asker prompt.Asker) (ResolvedAnswers, error) {
	var answers ResolvedAnswers
	var stored hoststate.Answers
	if opts.AnswersPath != "" {
		data, err := os.ReadFile(opts.AnswersPath)
		if err == nil {
			_ = json.Unmarshal(data, &stored)
		}
	}
	paths := harnessPathsFromOpts(opts)
	switch opts.Harness {
	case HarnessClaudeCode:
		current, _ := readDenyRules(paths.SettingsFile)
		if len(appendMissingRules(current, sagDenyRules)) > len(current) {
			storedVal := ""
			if stored.DenyRules != nil {
				storedVal = stored.DenyRules[paths.SettingsFile]
			}
			question := fmt.Sprintf("Add 3 deny rules to %s? [y/N] ", paths.SettingsFile)
			val, err := prompt.ResolveYesNo(asker, opts.DenyRules, storedVal, question, "--deny-rules")
			if err != nil {
				return ResolvedAnswers{}, err
			}
			answers.DenyRules = val
			if opts.AnswersPath != "" && val != storedVal {
				answers.pending = append(answers.pending, func() { saveAnswerDenyRules(opts.AnswersPath, paths.SettingsFile, val) })
			}
		} else {
			answers.DenyRules = opts.DenyRules
		}
	case HarnessPi:
		shell := opts.Env["SHELL"]
		zdotdir := opts.Env["ZDOTDIR"]
		profile, ok := ShellProfile(opts.GOOS, shell, zdotdir, opts.Home)
		if !ok {
			question := fmt.Sprintf("Add \"export NODE_EXTRA_CA_CERTS=%s/ca.pem\" to the profile of %s? [y/N] ", opts.CfgDir, shellLabel(shell))
			val, err := prompt.ResolveYesNo(asker, opts.ShellProfile, "", question, "--shell-profile")
			if err != nil {
				return ResolvedAnswers{}, err
			}
			answers.ShellProfile = val
		} else if !fileContainsLine(profile, `export NODE_EXTRA_CA_CERTS="`+caFilePath(opts)+`"`) {
			storedVal := ""
			if stored.ShellProfile != nil {
				storedVal = stored.ShellProfile[profile]
			}
			question := fmt.Sprintf("Add \"export NODE_EXTRA_CA_CERTS=%s/ca.pem\" to %s? [y/N] ", opts.CfgDir, profile)
			val, err := prompt.ResolveYesNo(asker, opts.ShellProfile, storedVal, question, "--shell-profile")
			if err != nil {
				return ResolvedAnswers{}, err
			}
			answers.ShellProfile = val
			if opts.AnswersPath != "" && val != storedVal {
				answers.pending = append(answers.pending, func() { saveAnswerShellProfile(opts.AnswersPath, profile, val) })
			}
		} else {
			answers.ShellProfile = opts.ShellProfile
		}

		guardPath := paths.GuardFile
		if guardPath != "" {
			if _, err := os.Stat(guardPath); os.IsNotExist(err) {
				storedVal := ""
				if stored.Guard != nil {
					storedVal = stored.Guard[guardPath]
				}
				question := fmt.Sprintf("Install %s to block SAG secret commands in bash? [y/N] ", guardPath)
				val, err := prompt.ResolveYesNo(asker, opts.Guard, storedVal, question, "--guard")
				if err != nil {
					return ResolvedAnswers{}, err
				}
				answers.Guard = val
				if opts.AnswersPath != "" && val != storedVal {
					answers.pending = append(answers.pending, func() { saveAnswerGuard(opts.AnswersPath, guardPath, val) })
				}
			} else {
				answers.Guard = opts.Guard
				if answers.Guard == "" && stored.Guard != nil {
					answers.Guard = stored.Guard[guardPath]
				}
				if opts.AnswersPath != "" && opts.Guard != "" && (stored.Guard == nil || stored.Guard[guardPath] != opts.Guard) {
					answers.pending = append(answers.pending, func() { saveAnswerGuard(opts.AnswersPath, guardPath, opts.Guard) })
				}
			}
		}
	}
	return answers, nil
}

func applyAnswers(opts *Options, answers ResolvedAnswers) {
	if answers.DenyRules != "" && opts.DenyRules == "" {
		opts.DenyRules = answers.DenyRules
	}
	if answers.ShellProfile != "" && opts.ShellProfile == "" {
		opts.ShellProfile = answers.ShellProfile
	}
	if answers.Guard != "" && opts.Guard == "" {
		opts.Guard = answers.Guard
	}
}

func saveAnswerDenyRules(answersPath, settingsFile, val string) {
	var a hoststate.Answers
	data, err := os.ReadFile(answersPath)
	if err == nil {
		_ = json.Unmarshal(data, &a)
	}
	if a.DenyRules == nil {
		a.DenyRules = map[string]string{}
	}
	a.DenyRules[settingsFile] = val
	raw, err := json.Marshal(a)
	if err != nil {
		return
	}
	_ = os.WriteFile(answersPath, raw, 0o600)
}

func saveAnswerShellProfile(answersPath, profile, val string) {
	var a hoststate.Answers
	data, err := os.ReadFile(answersPath)
	if err == nil {
		_ = json.Unmarshal(data, &a)
	}
	if a.ShellProfile == nil {
		a.ShellProfile = map[string]string{}
	}
	a.ShellProfile[profile] = val
	raw, err := json.Marshal(a)
	if err != nil {
		return
	}
	_ = os.WriteFile(answersPath, raw, 0o600)
}

func saveAnswerGuard(answersPath, guardPath, val string) {
	var a hoststate.Answers
	data, err := os.ReadFile(answersPath)
	if err == nil {
		_ = json.Unmarshal(data, &a)
	}
	if a.Guard == nil {
		a.Guard = map[string]string{}
	}
	a.Guard[guardPath] = val
	raw, err := json.Marshal(a)
	if err != nil {
		return
	}
	_ = os.WriteFile(answersPath, raw, 0o600)
}

func checkKeptRemoteEntry(opts Options) string {
	paths := harnessPathsFromOpts(opts)
	jwtStr, entryURL := readKeptEntry(paths.MCPFile, opts.Harness)
	if jwtStr == "" || entryURL != opts.URL {
		return ""
	}
	if subFromRawJWT(jwtStr) != opts.Sub {
		return ""
	}
	return jwtStr
}

func planConflictsWithErr(opts Options, mode Mode, containerRuns bool) ([]string, error) {
	conflicts := planConflicts(opts, mode, containerRuns)
	if len(conflicts) > 0 {
		return conflicts, &ConflictError{conflicts: conflicts, caTrust: deferredCATrust(opts)}
	}
	return nil, nil
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

	if c := checkMCPFile(paths.MCPFile, opts.URL); c != "" {
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

	if c := checkCATrustConflict(opts, paths); c != "" && !opts.DeferCATrust {
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

func checkMCPFile(path, wantURL string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(data, &top); err != nil || top == nil {
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
	if wantURL != "" && entryURL != wantURL {
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
	if auth == "" {
		return ""
	}
	if !strings.HasPrefix(auth, "Bearer ") {
		return "mcp-server sag in " + path + " has another form; remove it"
	}
	token := strings.TrimPrefix(auth, "Bearer ")
	if !isJWT(token) {
		return "mcp-server sag in " + path + " has another form; remove it"
	}
	return ""
}

func isJWT(s string) bool {
	parts := strings.Split(s, ".")
	return len(parts) == 3
}

func checkJSONFile(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ""
		}
		return "settings " + path + " cannot be read"
	}
	if len(data) == 0 {
		return ""
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(data, &top); err != nil || top == nil {
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

func deferredCATrust(opts Options) *CATrustConflictError {
	if opts.DeferCATrust {
		return nil
	}
	return caTrustConflict(opts, harnessPathsFromOpts(opts))
}

func shellLabel(shell string) string {
	if shell == "" {
		return "an unknown shell"
	}
	return shell
}
