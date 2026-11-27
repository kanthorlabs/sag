package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kanthorlabs/sag/internal/agentsetup"
	"github.com/kanthorlabs/sag/internal/hoststate"
	"github.com/kanthorlabs/sag/internal/prompt"
	"github.com/kanthorlabs/sag/internal/stepline"
)

func agentTestDeps(dir string) Deps {
	d := baseDeps(dir)
	d.HarnessInPATH = func(h agentsetup.Harness, environ map[string]string) bool { return false }
	d.Environ = func() map[string]string { return map[string]string{} }
	d.AgentRun = func(ctx context.Context, opts agentsetup.Options) error { return nil }
	return d
}

func TestAskPerDetectedHarness(t *testing.T) {
	dir := t.TempDir()
	claudeDir := filepath.Join(dir, "claude")
	piDir := filepath.Join(dir, "pi")

	d := agentTestDeps(dir)
	d.HarnessInPATH = func(h agentsetup.Harness, environ map[string]string) bool { return true }
	d.Environ = func() map[string]string {
		return map[string]string{
			"CLAUDE_CONFIG_DIR":   claudeDir,
			"PI_CODING_AGENT_DIR": piDir,
		}
	}

	askedSet := make(map[string]bool)
	d.ResolveYesNo = func(a prompt.Asker, flagValue, stored, question, flagName string) (string, error) {
		if stored != "" {
			return stored, nil
		}
		if flagName == "--agent-setup" {
			askedSet[question] = true
			return "no", nil
		}
		if flagValue != "" {
			return flagValue, nil
		}
		return "yes", nil
	}

	if err := Run(context.Background(), Flags{Autostart: "yes"}, d); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(askedSet) != 2 {
		t.Fatalf("unique agent-setup questions = %d; want 2; got: %v", len(askedSet), askedSet)
	}
}

func TestAskPerDetectedHarnessByDir(t *testing.T) {
	dir := t.TempDir()
	claudeDir1 := filepath.Join(dir, "claude1")
	claudeDir2 := filepath.Join(dir, "claude2")

	var savedAnswers hoststate.Answers

	d := agentTestDeps(dir)
	d.HarnessInPATH = func(h agentsetup.Harness, environ map[string]string) bool {
		return h == agentsetup.HarnessClaudeCode
	}
	d.LoadAnswers = func(p hoststate.Paths) (hoststate.Answers, error) {
		key := agentSetupKey(agentsetup.HarnessClaudeCode, claudeDir1)
		return hoststate.Answers{
			AgentSetup: map[string]string{key: "yes"},
		}, nil
	}
	d.SaveAnswers = func(p hoststate.Paths, a hoststate.Answers) error {
		savedAnswers = a
		return nil
	}
	d.Environ = func() map[string]string {
		return map[string]string{"CLAUDE_CONFIG_DIR": claudeDir2}
	}

	askedQuestions := 0
	d.ResolveYesNo = func(a prompt.Asker, flagValue, stored, question, flagName string) (string, error) {
		if stored != "" {
			return stored, nil
		}
		if flagName == "--agent-setup" {
			askedQuestions++
			return "no", nil
		}
		if flagValue != "" {
			return flagValue, nil
		}
		return "yes", nil
	}

	if err := Run(context.Background(), Flags{Autostart: "yes"}, d); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if askedQuestions == 0 {
		t.Fatal("expected question for new dir (claude2); none asked")
	}
	key2 := agentSetupKey(agentsetup.HarnessClaudeCode, claudeDir2)
	if savedAnswers.AgentSetup[key2] == "" {
		t.Fatalf("answer for new dir not saved; keys: %v", savedAnswers.AgentSetup)
	}
	key1 := agentSetupKey(agentsetup.HarnessClaudeCode, claudeDir1)
	if savedAnswers.AgentSetup[key1] != "yes" {
		t.Fatalf("existing answer for dir1 lost; keys: %v", savedAnswers.AgentSetup)
	}
}

func TestNoTerminalNamesAgentSetup(t *testing.T) {
	dir := t.TempDir()
	claudeDir := filepath.Join(dir, "claude")

	d := agentTestDeps(dir)
	d.HarnessInPATH = func(h agentsetup.Harness, environ map[string]string) bool {
		return h == agentsetup.HarnessClaudeCode
	}
	d.Environ = func() map[string]string {
		return map[string]string{"CLAUDE_CONFIG_DIR": claudeDir}
	}
	d.ResolveYesNo = func(a prompt.Asker, flagValue, stored, question, flagName string) (string, error) {
		if stored != "" {
			return stored, nil
		}
		if flagName == "--agent-setup" {
			return "", &prompt.RefusalError{Flag: "--agent-setup"}
		}
		if flagValue != "" {
			return flagValue, nil
		}
		return "yes", nil
	}

	err := Run(context.Background(), Flags{Autostart: "yes"}, d)
	if err == nil {
		t.Fatal("Run: want error, got nil")
	}
	var refusal *prompt.RefusalError
	if !errors.As(err, &refusal) {
		t.Fatalf("error type = %T; want *prompt.RefusalError", err)
	}
	if refusal.Flag != "--agent-setup" {
		t.Fatalf("refusal.Flag = %q; want --agent-setup", refusal.Flag)
	}
}

func TestCATrustConflictExit1AfterSteps(t *testing.T) {
	dir := t.TempDir()
	claudeDir := filepath.Join(dir, "claude")
	piDir := filepath.Join(dir, "pi")

	var ranHarnesses []string
	d := agentTestDeps(dir)
	d.HarnessInPATH = func(h agentsetup.Harness, environ map[string]string) bool { return true }
	d.Environ = func() map[string]string {
		return map[string]string{
			"CLAUDE_CONFIG_DIR":   claudeDir,
			"PI_CODING_AGENT_DIR": piDir,
		}
	}
	d.AgentRun = func(ctx context.Context, opts agentsetup.Options) error {
		ranHarnesses = append(ranHarnesses, string(opts.Harness))
		if opts.Harness == agentsetup.HarnessClaudeCode {
			return errCATrustConflict
		}
		return nil
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

	if len(ranHarnesses) != 2 {
		t.Fatalf("ran %d harnesses; want 2 (both attempted)", len(ranHarnesses))
	}

	piFound := false
	for _, h := range ranHarnesses {
		if h == string(agentsetup.HarnessPi) {
			piFound = true
		}
	}
	if !piFound {
		t.Fatal("pi was not attempted after claude-code CA conflict")
	}
}

func TestStep14RunsUnderLock(t *testing.T) {
	dir := t.TempDir()
	claudeDir := filepath.Join(dir, "claude")
	cfgDir := filepath.Join(dir, "sag")

	if err := os.MkdirAll(cfgDir, 0o700); err != nil {
		t.Fatal(err)
	}

	var underLockDuringRun bool
	lockAcquired := false

	d := agentTestDeps(dir)
	d.ResolvePaths = func() (hoststate.Paths, error) {
		return hoststate.Paths{
			Cfg:         cfgDir,
			SAGYaml:     filepath.Join(cfgDir, "sag.yaml"),
			CAPem:       filepath.Join(cfgDir, "ca.pem"),
			AnswersJSON: filepath.Join(cfgDir, "answers.json"),
			Lock:        filepath.Join(cfgDir, "lock"),
		}, nil
	}
	d.EnsureCfg = func(p hoststate.Paths) (bool, error) { return false, nil }
	d.AcquireLock = func(p hoststate.Paths) (*hoststate.Lock, error) {
		l, err := hoststate.AcquireLock(p)
		if err == nil {
			lockAcquired = true
		}
		return l, err
	}
	d.ReleaseLock = func(l *hoststate.Lock) error { return l.Release() }

	d.HarnessInPATH = func(h agentsetup.Harness, environ map[string]string) bool {
		return h == agentsetup.HarnessClaudeCode
	}
	d.Environ = func() map[string]string {
		return map[string]string{"CLAUDE_CONFIG_DIR": claudeDir}
	}
	d.AgentRun = func(ctx context.Context, opts agentsetup.Options) error {
		underLockDuringRun = opts.UnderLock && lockAcquired
		return nil
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

	if err := Run(context.Background(), Flags{Autostart: "yes"}, d); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if !underLockDuringRun {
		t.Fatal("step 14 did not run with UnderLock=true while lock was held")
	}
}

func fakeDetected(harnesses ...agentsetup.Harness) []harnessDet {
	var result []harnessDet
	for _, h := range harnesses {
		result = append(result, harnessDet{harness: h, sub: string(h)})
	}
	return result
}

var _ io.Writer = io.Discard
var _ stepline.Writer = stepline.New(io.Discard)

func TestHarnessConfigDirIsAbsolute(t *testing.T) {
	environ := map[string]string{"CLAUDE_CONFIG_DIR": "relative/path"}
	dir := harnessConfigDir(agentsetup.HarnessClaudeCode, environ, "/home/user")
	if !filepath.IsAbs(dir) {
		t.Errorf("harnessConfigDir = %q; want absolute path", dir)
	}
}

func TestConvertAgentRunErrorTyped(t *testing.T) {
	catErr := &agentsetup.CATrustConflictError{
		Current:    "/other/path.pem",
		CAPath:     "/sag/ca.pem",
		BundlePath: "/sag/ca-bundle.pem",
		Source:     "the environment",
	}
	got := convertAgentRunError(catErr)
	if !errors.Is(got, errCATrustConflict) {
		t.Errorf("convertAgentRunError(*CATrustConflictError) = %v; want errCATrustConflict", got)
	}
}

func TestConvertAgentRunErrorPlainNotConverted(t *testing.T) {
	plainErr := errors.New("fail ca-trust NODE_EXTRA_CA_CERTS is /other.pem; run: cat ...")
	got := convertAgentRunError(plainErr)
	if errors.Is(got, errCATrustConflict) {
		t.Error("plain error with CA trust message should not be converted to errCATrustConflict")
	}
}

func TestPlanOnlyCalledForYesHarnesses(t *testing.T) {
	dir := t.TempDir()
	claudeDir := filepath.Join(dir, "claude")
	piDir := filepath.Join(dir, "pi")

	var planOnlyCalls []agentsetup.Harness
	d := agentTestDeps(dir)
	d.HarnessInPATH = func(h agentsetup.Harness, environ map[string]string) bool { return true }
	d.Environ = func() map[string]string {
		return map[string]string{
			"CLAUDE_CONFIG_DIR":   claudeDir,
			"PI_CODING_AGENT_DIR": piDir,
		}
	}
	d.PlanOnly = func(ctx context.Context, opts agentsetup.Options) (agentsetup.Plan, error) {
		planOnlyCalls = append(planOnlyCalls, opts.Harness)
		return agentsetup.Plan{}, nil
	}
	d.ResolveYesNo = func(a prompt.Asker, flagValue, stored, question, flagName string) (string, error) {
		if stored != "" {
			return stored, nil
		}
		if flagName == "--agent-setup" {
			if strings.Contains(question, string(agentsetup.HarnessClaudeCode)) {
				return "yes", nil
			}
			return "no", nil
		}
		if flagValue != "" {
			return flagValue, nil
		}
		return "yes", nil
	}

	if err := Run(context.Background(), Flags{Autostart: "yes"}, d); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(planOnlyCalls) != 1 {
		t.Fatalf("PlanOnly calls = %d; want 1 (only claude-code=yes)", len(planOnlyCalls))
	}
	if planOnlyCalls[0] != agentsetup.HarnessClaudeCode {
		t.Errorf("PlanOnly called for %v; want claude-code", planOnlyCalls[0])
	}
}

func TestPlanOnlyConflictExitsBeforeLock(t *testing.T) {
	dir := t.TempDir()
	claudeDir := filepath.Join(dir, "claude")

	lockAcquired := false
	d := agentTestDeps(dir)
	d.AcquireLock = func(p hoststate.Paths) (*hoststate.Lock, error) {
		lockAcquired = true
		return nil, errors.New("should not reach lock")
	}
	d.HarnessInPATH = func(h agentsetup.Harness, environ map[string]string) bool {
		return h == agentsetup.HarnessClaudeCode
	}
	d.Environ = func() map[string]string {
		return map[string]string{"CLAUDE_CONFIG_DIR": claudeDir}
	}
	conflictErr := fmt.Errorf("disk check failed")
	d.PlanOnly = func(ctx context.Context, opts agentsetup.Options) (agentsetup.Plan, error) {
		return agentsetup.Plan{}, conflictErr
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
		t.Fatal("Run: want error from PlanOnly conflict, got nil")
	}
	if !errors.Is(err, conflictErr) {
		t.Errorf("error = %v; want conflictErr", err)
	}
	if lockAcquired {
		t.Error("lock was acquired after PlanOnly conflict; want exit before step 4")
	}
}

func TestPlanOnlyAnswersPassedToStep14(t *testing.T) {
	dir := t.TempDir()
	claudeDir := filepath.Join(dir, "claude")
	cfgDir := filepath.Join(dir, "sag")
	if err := os.MkdirAll(cfgDir, 0o700); err != nil {
		t.Fatal(err)
	}

	resolvedAnswers := agentsetup.ResolvedAnswers{DenyRules: "yes"}

	d := agentTestDeps(dir)
	d.ResolvePaths = func() (hoststate.Paths, error) {
		return hoststate.Paths{
			Cfg:         cfgDir,
			AnswersJSON: filepath.Join(cfgDir, "answers.json"),
			Lock:        filepath.Join(cfgDir, "lock"),
		}, nil
	}
	d.EnsureCfg = func(p hoststate.Paths) (bool, error) { return false, nil }
	d.AcquireLock = func(p hoststate.Paths) (*hoststate.Lock, error) {
		return hoststate.AcquireLock(p)
	}
	d.ReleaseLock = func(l *hoststate.Lock) error { return l.Release() }
	d.HarnessInPATH = func(h agentsetup.Harness, environ map[string]string) bool {
		return h == agentsetup.HarnessClaudeCode
	}
	d.Environ = func() map[string]string {
		return map[string]string{"CLAUDE_CONFIG_DIR": claudeDir}
	}
	d.PlanOnly = func(ctx context.Context, opts agentsetup.Options) (agentsetup.Plan, error) {
		return agentsetup.Plan{Answers: resolvedAnswers}, nil
	}
	var runOpts agentsetup.Options
	d.AgentRun = func(ctx context.Context, opts agentsetup.Options) error {
		runOpts = opts
		return nil
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

	if err := Run(context.Background(), Flags{Autostart: "yes"}, d); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if runOpts.DenyRules != "yes" {
		t.Errorf("step 14 DenyRules = %q; want %q (from step 3 answers)", runOpts.DenyRules, "yes")
	}
}
