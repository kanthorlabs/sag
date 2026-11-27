package agentsetup

import (
	"context"
	"fmt"

	"github.com/kanthorlabs/sag/internal/prompt"
)

type Harness string

const (
	HarnessClaudeCode Harness = "claude-code"
	HarnessPi         Harness = "pi"
)

type Mode int

const (
	ModeLocal Mode = iota
	ModeRemote
)

type Options struct {
	Harness      Harness
	Sub          string
	URL          string
	CfgDir       string
	PodmanPath   string
	Env          map[string]string
	Home         string
	GOOS         string
	DenyRules    string
	ShellProfile string
	Guard        string
	UnderLock    bool
	AnswersPath  string
	Asker        prompt.Asker
}

type ResolvedAnswers struct {
	DenyRules    string
	ShellProfile string
	Guard        string
}

type Plan struct {
	Answers   ResolvedAnswers
	Conflicts []string
}

type CATrustConflictError struct {
	Current    string
	CAPath     string
	BundlePath string
	Source     string
}

func (e *CATrustConflictError) Error() string {
	return fmt.Sprintf("fail ca-trust NODE_EXTRA_CA_CERTS is %s; run: cat %q %q > %q; then set NODE_EXTRA_CA_CERTS to %s in %s",
		e.Current, e.Current, e.CAPath, e.BundlePath, e.BundlePath, e.Source)
}

type ConflictError struct {
	conflicts []string
}

func (e *ConflictError) Error() string {
	if len(e.conflicts) == 1 {
		return e.conflicts[0]
	}
	return fmt.Sprintf("%d plan conflicts", len(e.conflicts))
}

func Run(ctx context.Context, opts Options) error {
	return run(ctx, opts)
}

func PlanOnly(ctx context.Context, opts Options) (Plan, error) {
	return planOnly(ctx, opts)
}
