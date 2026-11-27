package agentsetup

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/kanthorlabs/sag/internal/prompt"
)

func piOpts(t *testing.T, shell string) Options {
	t.Helper()
	dir := t.TempDir()
	env := map[string]string{
		"PI_CODING_AGENT_DIR": filepath.Join(dir, "pi"),
		"PATH":                "/usr/bin:/bin",
	}
	if shell != "" {
		env["SHELL"] = shell
	}
	return Options{
		Harness:     HarnessPi,
		Sub:         "pi",
		URL:         "https://localhost:8443/mcp",
		CfgDir:      dir,
		PodmanPath:  "podman",
		Env:         env,
		Home:        dir,
		GOOS:        "darwin",
		AnswersPath: filepath.Join(dir, "answers.json"),
	}
}

func TestPiUnknownShellNeedsShellProfileAnswer(t *testing.T) {
	for _, shell := range []string{"", "/usr/bin/fish"} {
		opts := piOpts(t, shell)
		_, err := resolveAnswers(opts, noTerminalAsker())
		var refusal *prompt.RefusalError
		if !errors.As(err, &refusal) {
			t.Fatalf("SHELL=%q: error = %v; want *prompt.RefusalError", shell, err)
		}
		if refusal.Flag != "--shell-profile" {
			t.Errorf("SHELL=%q: flag = %q; want --shell-profile", shell, refusal.Flag)
		}
	}
}

func TestPiGuardFlagIsStoredAndReusedOnSecondRun(t *testing.T) {
	opts := piOpts(t, "/bin/zsh")
	opts.ShellProfile = "yes"
	opts.Guard = "yes"
	first, err := resolveAnswers(opts, noTerminalAsker())
	if err != nil {
		t.Fatalf("first run: %v", err)
	}
	if first.Guard != "yes" {
		t.Fatalf("first run guard = %q; want yes", first.Guard)
	}
	first.save()

	guardPath := harnessPathsFromOpts(opts).GuardFile
	if err := os.MkdirAll(filepath.Dir(guardPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(guardPath, []byte(guardMarker+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	opts.ShellProfile = ""
	opts.Guard = ""
	second, err := resolveAnswers(opts, noTerminalAsker())
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if second.Guard != "yes" {
		t.Errorf("second run guard = %q; want the stored yes", second.Guard)
	}
}

func TestRemoteRunWithoutTerminalIsNoTerminalBeforeLock(t *testing.T) {
	home := t.TempDir()
	cfgDir := filepath.Join(home, "cfg")
	if err := os.MkdirAll(cfgDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "ca.pem"), []byte("fake-ca"), 0o600); err != nil {
		t.Fatal(err)
	}
	binDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(binDir, "claude"), []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	claudeDir := filepath.Join(home, "claude")

	oldOpen := remoteOpenTTY
	defer func() { remoteOpenTTY = oldOpen }()
	remoteOpenTTY = func() (*os.File, error) { return nil, errors.New("no terminal") }

	t.Setenv("HOME", filepath.Join(home, "no-such-home"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "no-such-xdg"))

	opts := Options{
		Harness:   HarnessClaudeCode,
		Sub:       "remote-a",
		URL:       "https://127.0.0.1:8443/mcp",
		CfgDir:    cfgDir,
		Home:      home,
		GOOS:      "darwin",
		DenyRules: "no",
		Env:       map[string]string{"CLAUDE_CONFIG_DIR": claudeDir, "PATH": binDir},
	}
	err := run(context.Background(), opts)
	if !errors.Is(err, prompt.ErrNoTerminal) {
		t.Fatalf("error = %v; want prompt.ErrNoTerminal", err)
	}
	if _, statErr := os.Stat(filepath.Join(home, "no-such-home")); statErr == nil {
		t.Error("run took the host lock before the terminal check")
	}
	if _, statErr := os.Stat(claudeDir); statErr == nil {
		t.Error("run created the harness directory")
	}
}

func TestRemoteRunWithoutTerminalWritesNoAnswers(t *testing.T) {
	home := t.TempDir()
	cfgDir := filepath.Join(home, "cfg")
	if err := os.MkdirAll(cfgDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "ca.pem"), []byte("fake-ca"), 0o600); err != nil {
		t.Fatal(err)
	}
	binDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(binDir, "claude"), []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}

	oldOpen := remoteOpenTTY
	defer func() { remoteOpenTTY = oldOpen }()
	remoteOpenTTY = func() (*os.File, error) { return nil, errors.New("no terminal") }

	answersPath := filepath.Join(cfgDir, "answers.json")
	opts := Options{
		Harness:     HarnessClaudeCode,
		Sub:         "remote-a",
		URL:         "https://127.0.0.1:8443/mcp",
		CfgDir:      cfgDir,
		Home:        home,
		GOOS:        "darwin",
		DenyRules:   "yes",
		UnderLock:   true,
		AnswersPath: answersPath,
		Env:         map[string]string{"CLAUDE_CONFIG_DIR": filepath.Join(home, "claude"), "PATH": binDir},
	}
	err := run(context.Background(), opts)
	if !errors.Is(err, prompt.ErrNoTerminal) {
		t.Fatalf("error = %v; want prompt.ErrNoTerminal", err)
	}
	if _, statErr := os.Stat(answersPath); statErr == nil {
		t.Error("run wrote answers.json on an exit in the plan")
	}
}
