package agentsetup

import (
	"context"
	"errors"
)

var errNotImplemented = errors.New("not implemented")

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
}

type Plan struct {
	Answers   map[string]string
	Conflicts []string
}

func Run(ctx context.Context, opts Options) error {
	return run(ctx, opts)
}

func PlanOnly(ctx context.Context, opts Options) (Plan, error) {
	return planOnly(ctx, opts)
}
