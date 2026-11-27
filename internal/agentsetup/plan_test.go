package agentsetup

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kanthorlabs/sag/internal/prompt"
	"github.com/kanthorlabs/sag/internal/stepline"
)

func baseOpts(t *testing.T) Options {
	t.Helper()
	dir := t.TempDir()
	return Options{
		Harness:    HarnessClaudeCode,
		Sub:        "claude-code",
		URL:        "https://localhost:8443/mcp",
		CfgDir:     dir,
		PodmanPath: "podman",
		Env: map[string]string{
			"CLAUDE_CONFIG_DIR": dir,
			"PATH":              "/usr/bin:/bin",
		},
		Home: dir,
		GOOS: "darwin",
	}
}

func hasConflict(conflicts []string, substr string) bool {
	for _, c := range conflicts {
		if strings.Contains(c, substr) {
			return true
		}
	}
	return false
}

func TestConflictHarnessNotInPATH(t *testing.T) {
	opts := baseOpts(t)
	conflicts := planConflicts(opts, ModeLocal, true)
	if !hasConflict(conflicts, "not in PATH") {
		t.Errorf("conflicts = %v; want 'not in PATH' conflict", conflicts)
	}
}

func TestConflictContainerNotRunning(t *testing.T) {
	opts := baseOpts(t)
	conflicts := planConflicts(opts, ModeLocal, false)
	if !hasConflict(conflicts, "does not run") {
		t.Errorf("conflicts = %v; want 'does not run' conflict", conflicts)
	}
}

func TestConflictContainerNotRunningSkippedWhenBootstrap(t *testing.T) {
	opts := baseOpts(t)
	opts.UnderLock = true
	conflicts := planConflicts(opts, ModeLocal, false)
	if hasConflict(conflicts, "does not run") {
		t.Error("conflicts should not include 'does not run' when under bootstrap lock")
	}
}

func TestConflictRemoteMissingCAPem(t *testing.T) {
	opts := baseOpts(t)
	opts.URL = "https://remote.example:8443/mcp"
	conflicts := planConflicts(opts, ModeRemote, false)
	if !hasConflict(conflicts, "ca.pem absent") {
		t.Errorf("conflicts = %v; want 'ca.pem absent' conflict for remote mode", conflicts)
	}
}

func TestConflictInvalidMCPFile(t *testing.T) {
	opts := baseOpts(t)
	dir := opts.Env["CLAUDE_CONFIG_DIR"]
	mcpFile := filepath.Join(dir, ".claude.json")
	os.WriteFile(mcpFile, []byte("not json"), 0o600)
	conflicts := planConflicts(opts, ModeLocal, true)
	if !hasConflict(conflicts, "not valid JSON") {
		t.Errorf("conflicts = %v; want 'not valid JSON' conflict", conflicts)
	}
}

func TestConflictCommandEntry(t *testing.T) {
	opts := baseOpts(t)
	dir := opts.Env["CLAUDE_CONFIG_DIR"]
	mcpFile := filepath.Join(dir, ".claude.json")
	entry := map[string]any{
		"mcpServers": map[string]any{
			"sag": map[string]any{
				"command": "sag",
				"url":     "https://localhost:8443/mcp",
			},
		},
	}
	data, _ := json.Marshal(entry)
	os.WriteFile(mcpFile, data, 0o600)
	conflicts := planConflicts(opts, ModeLocal, true)
	if !hasConflict(conflicts, "another form") {
		t.Errorf("conflicts = %v; want 'another form' conflict for command entry", conflicts)
	}
}

func TestConflictSkillWithoutMarker(t *testing.T) {
	opts := baseOpts(t)
	dir := opts.Env["CLAUDE_CONFIG_DIR"]
	skillPath := filepath.Join(dir, "skills", "sag", "SKILL.md")
	os.MkdirAll(filepath.Dir(skillPath), 0o700)
	os.WriteFile(skillPath, []byte("# Custom skill\nno marker\n"), 0o644)
	conflicts := planConflicts(opts, ModeLocal, true)
	if !hasConflict(conflicts, "managed marker") {
		t.Errorf("conflicts = %v; want 'managed marker' conflict for skill without marker", conflicts)
	}
}

func TestConflictGuardWithoutMarker(t *testing.T) {
	opts := baseOpts(t)
	opts.Harness = HarnessPi
	piDir := t.TempDir()
	opts.Env["PI_CODING_AGENT_DIR"] = piDir
	guardPath := filepath.Join(piDir, "extensions", "sag-guard.ts")
	os.MkdirAll(filepath.Dir(guardPath), 0o700)
	os.WriteFile(guardPath, []byte("// custom guard\n"), 0o644)
	conflicts := planConflicts(opts, ModeLocal, true)
	if !hasConflict(conflicts, "managed marker") {
		t.Errorf("conflicts = %v; want 'managed marker' conflict for guard without marker", conflicts)
	}
}

func TestConflictFishWithShellProfile(t *testing.T) {
	opts := baseOpts(t)
	opts.Harness = HarnessPi
	opts.ShellProfile = "yes"
	opts.Env["SHELL"] = "/usr/local/bin/fish"
	piDir := t.TempDir()
	opts.Env["PI_CODING_AGENT_DIR"] = piDir
	conflicts := planConflicts(opts, ModeLocal, true)
	if !hasConflict(conflicts, "unsupported shell") {
		t.Errorf("conflicts = %v; want 'unsupported shell' conflict for fish with --shell-profile yes", conflicts)
	}
}

func noTerminalAsker() prompt.Asker {
	return prompt.Asker{Open: func() (io.ReadWriteCloser, error) {
		return nil, errors.New("no terminal")
	}}
}

type fakeTTY struct {
	answer string
}

func (f *fakeTTY) Read(p []byte) (n int, err error) {
	src := []byte(f.answer + "\n")
	n = copy(p, src)
	f.answer = ""
	if n == 0 {
		return 0, io.EOF
	}
	return n, nil
}
func (f *fakeTTY) Write(p []byte) (n int, err error) { return len(p), nil }
func (f *fakeTTY) Close() error                      { return nil }

func yesAsker() prompt.Asker {
	return prompt.Asker{Open: func() (io.ReadWriteCloser, error) {
		return &fakeTTY{answer: "y"}, nil
	}}
}

func TestB82NeededAnswerNoTerminalGivesRefusal(t *testing.T) {
	opts := baseOpts(t)
	opts.Asker = noTerminalAsker()
	opts.AnswersPath = filepath.Join(t.TempDir(), "answers.json")

	_, err := resolveAnswers(opts, opts.Asker)
	if err == nil {
		t.Fatal("B82: resolveAnswers returned nil; want *prompt.RefusalError")
	}
	var refusal *prompt.RefusalError
	if !errors.As(err, &refusal) {
		t.Fatalf("B82: error type = %T; want *prompt.RefusalError", err)
	}
	if !strings.Contains(refusal.Flag, "--deny-rules") {
		t.Errorf("B82: RefusalError.Flag = %q; want --deny-rules", refusal.Flag)
	}
}

func TestB82AnswerSavedToAnswersJSON(t *testing.T) {
	opts := baseOpts(t)
	answersPath := filepath.Join(t.TempDir(), "answers.json")
	opts.AnswersPath = answersPath

	answers, err := resolveAnswers(opts, yesAsker())
	if err != nil {
		t.Fatalf("B82: resolveAnswers: %v", err)
	}
	if answers.DenyRules != "yes" {
		t.Errorf("B82: DenyRules = %q; want yes", answers.DenyRules)
	}

	data, err := os.ReadFile(answersPath)
	if err != nil {
		t.Fatalf("B82: answers.json not written: %v", err)
	}
	paths := harnessPathsFromOpts(opts)
	if !strings.Contains(string(data), paths.SettingsFile) {
		t.Errorf("B82: answers.json %q missing settings path %q", data, paths.SettingsFile)
	}
}

func TestB82StoredAnswerNoRePrompt(t *testing.T) {
	opts := baseOpts(t)
	answersPath := filepath.Join(t.TempDir(), "answers.json")
	opts.AnswersPath = answersPath

	_, err := resolveAnswers(opts, yesAsker())
	if err != nil {
		t.Fatalf("B82 setup: %v", err)
	}

	opts2 := baseOpts(t)
	opts2.CfgDir = opts.CfgDir
	opts2.Env = opts.Env
	opts2.AnswersPath = answersPath

	answers2, err2 := resolveAnswers(opts2, noTerminalAsker())
	if err2 != nil {
		t.Fatalf("B82: second resolveAnswers with stored answer: %v", err2)
	}
	if answers2.DenyRules != "yes" {
		t.Errorf("B82: second run DenyRules = %q; want yes from stored answer", answers2.DenyRules)
	}
}

func TestB82GuardAnswerResolvedForPi(t *testing.T) {
	opts := baseOpts(t)
	opts.Harness = HarnessPi
	piDir := t.TempDir()
	opts.Env = map[string]string{
		"PI_CODING_AGENT_DIR": piDir,
		"SHELL":               "/bin/zsh",
		"PATH":                "/usr/bin:/bin",
	}
	opts.Home = t.TempDir()
	opts.Guard = "yes"
	opts.ShellProfile = "no"
	opts.AnswersPath = filepath.Join(t.TempDir(), "answers.json")

	answers, err := resolveAnswers(opts, noTerminalAsker())
	if err != nil {
		t.Fatalf("B82: guard resolveAnswers: %v", err)
	}
	if answers.Guard != "yes" {
		t.Errorf("B82: Guard = %q; want yes", answers.Guard)
	}
}

func TestB82PromptTextsDenyRules(t *testing.T) {
	opts := baseOpts(t)
	paths := harnessPathsFromOpts(opts)
	wantQ := "Add 3 deny rules to " + paths.SettingsFile + "? [y/N] "
	gotQ := fmt.Sprintf("Add 3 deny rules to %s? [y/N] ", paths.SettingsFile)
	if gotQ != wantQ {
		t.Errorf("B82: deny rules prompt = %q; want %q", gotQ, wantQ)
	}
}

func TestB86KeptRemoteEntryPlanPasses(t *testing.T) {
	dir := t.TempDir()
	mcpFile := filepath.Join(dir, ".claude.json")
	keptJWT := makeTestJWT("claude-code")
	url := "https://127.0.0.1:8443/mcp"

	entry := map[string]any{
		"mcpServers": map[string]any{
			"sag": map[string]any{
				"type": "http",
				"url":  url,
				"headers": map[string]string{
					"Authorization": "Bearer " + keptJWT,
				},
			},
		},
	}
	data, _ := json.Marshal(entry)
	os.WriteFile(mcpFile, data, 0o600)

	oldOpen := remoteOpenTTY
	defer func() { remoteOpenTTY = oldOpen }()
	remoteOpenTTY = func() (*os.File, error) {
		return nil, errors.New("no terminal")
	}

	caPath := filepath.Join(dir, "ca.pem")
	os.WriteFile(caPath, []byte("fake-ca"), 0o600)

	settingsFile := filepath.Join(dir, "settings.json")
	sagDenyAll := []string{}
	for _, r := range sagDenyRules {
		sagDenyAll = append(sagDenyAll, r)
	}
	settingsData, _ := json.Marshal(map[string]any{
		"permissions": map[string]any{"deny": sagDenyAll},
	})
	os.WriteFile(settingsFile, settingsData, 0o600)

	opts := Options{
		Harness:    HarnessClaudeCode,
		Sub:        "claude-code",
		URL:        url,
		CfgDir:     dir,
		Env:        map[string]string{"CLAUDE_CONFIG_DIR": dir},
		Home:       dir,
		PodmanPath: "podman",
		GOOS:       "darwin",
		DenyRules:  "yes",
	}

	plan, err := planOnly(context.Background(), opts)
	if err != nil {
		t.Fatalf("B86: planOnly returned error for kept entry: %v", err)
	}
	_ = plan
}

func TestB88EnvAcceptedSettingsKeyAbsent(t *testing.T) {
	opts := baseOpts(t)
	caPath := caFilePath(opts)
	os.WriteFile(caPath, []byte("fake-ca"), 0o600)
	opts.Env["NODE_EXTRA_CA_CERTS"] = caPath

	var outBuf strings.Builder
	w := stepline.New(&outBuf)
	err := applyCATrustClaudeCode(opts, caPath, w)
	if err != nil {
		t.Fatalf("B88: applyCATrustClaudeCode with env=caPath: %v", err)
	}
	paths := ClaudeCodePaths(opts.Env["CLAUDE_CONFIG_DIR"], opts.Home)
	data, err := os.ReadFile(paths.SettingsFile)
	if err != nil {
		t.Fatalf("B88: settings.json not written: %v", err)
	}
	if !strings.Contains(string(data), "NODE_EXTRA_CA_CERTS") {
		t.Errorf("B88: settings.json %q: missing NODE_EXTRA_CA_CERTS key", data)
	}
}

func TestB89AnotherURLConflict(t *testing.T) {
	dir := t.TempDir()
	mcpFile := filepath.Join(dir, ".claude.json")
	entry := map[string]any{
		"mcpServers": map[string]any{
			"sag": map[string]any{
				"type": "http",
				"url":  "https://other.example:8443/mcp",
				"headers": map[string]string{
					"Authorization": "Bearer " + makeTestJWT("x"),
				},
			},
		},
	}
	data, _ := json.Marshal(entry)
	os.WriteFile(mcpFile, data, 0o600)

	conflict := checkMCPFile(mcpFile, "https://localhost:8443/mcp")
	if conflict == "" {
		t.Fatal("B89: want conflict for different url, got empty")
	}
	if !strings.Contains(conflict, "another form") {
		t.Errorf("B89: conflict = %q; want 'another form'", conflict)
	}
}

func TestB89BearerNotJWT(t *testing.T) {
	dir := t.TempDir()
	mcpFile := filepath.Join(dir, ".claude.json")
	entry := map[string]any{
		"mcpServers": map[string]any{
			"sag": map[string]any{
				"type": "http",
				"url":  "https://localhost:8443/mcp",
				"headers": map[string]string{
					"Authorization": "Bearer not-a-jwt",
				},
			},
		},
	}
	data, _ := json.Marshal(entry)
	os.WriteFile(mcpFile, data, 0o600)

	conflict := checkMCPFile(mcpFile, "https://localhost:8443/mcp")
	if conflict == "" {
		t.Fatal("B89: want conflict for non-JWT bearer, got empty")
	}
	if !strings.Contains(conflict, "another form") {
		t.Errorf("B89: conflict = %q; want 'another form'", conflict)
	}
}

func TestB90FishNoShellProfileNoError(t *testing.T) {
	opts := baseOpts(t)
	opts.Harness = HarnessPi
	opts.ShellProfile = "no"
	opts.GOOS = "darwin"
	opts.Env["SHELL"] = "/usr/local/bin/fish"
	piDir := t.TempDir()
	opts.Env["PI_CODING_AGENT_DIR"] = piDir

	if err := applyShellProfile(context.Background(), opts); err != nil {
		t.Fatalf("B90: applyShellProfile with fish --shell-profile no: %v", err)
	}
	profilePath := filepath.Join(opts.Home, ".zshrc")
	if _, err := os.Stat(profilePath); !os.IsNotExist(err) {
		t.Error("B90: profile file created for fish with --shell-profile no")
	}
}

func TestB93UnreadableFile(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root bypasses mode 0000")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	os.WriteFile(path, []byte(`{"key":"value"}`), 0o000)
	defer os.Chmod(path, 0o600)

	conflict := checkJSONFile(path)
	if conflict == "" {
		t.Fatal("B93: want conflict for unreadable file, got empty")
	}
	if !strings.Contains(conflict, "cannot be read") {
		t.Errorf("B93: conflict = %q; want 'cannot be read'", conflict)
	}
}

func TestB93TopLevelNull(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	os.WriteFile(path, []byte("null"), 0o600)

	conflict := checkJSONFile(path)
	if conflict == "" {
		t.Fatal("B93: want conflict for null top-level, got empty")
	}
}

func TestB93TopLevelArray(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	os.WriteFile(path, []byte("[]"), 0o600)

	conflict := checkJSONFile(path)
	if conflict == "" {
		t.Fatal("B93: want conflict for array top-level, got empty")
	}
}

func TestB93MCPFileNullTopLevel(t *testing.T) {
	dir := t.TempDir()
	mcpFile := filepath.Join(dir, ".claude.json")
	os.WriteFile(mcpFile, []byte("null"), 0o600)

	conflict := checkMCPFile(mcpFile, "https://localhost:8443/mcp")
	if conflict == "" {
		t.Fatal("B93: want conflict for null MCP top-level, got empty")
	}
}
