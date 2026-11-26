package bootstrap

import (
	"context"
	"errors"

	"github.com/kanthorlabs/sag/internal/hoststate"
	"github.com/kanthorlabs/sag/internal/prompt"
	"github.com/kanthorlabs/sag/internal/stepline"
)

var errNotImplemented = errors.New("not implemented")

type Flags struct {
	AutoApprove  []string
	Autostart    string
	AgentSetup   map[string]string
	DenyRules    string
	ShellProfile string
	Guard        string
}

type State struct {
	Flags      Flags
	Paths      hoststate.Paths
	Answers    hoststate.Answers
	Lock       *hoststate.Lock
	Asker      prompt.Asker
	Out        stepline.Writer
	PodmanPath string
	SAGPath    string
	UID        string
	Home       string
	CfgCreated bool
	Changed    bool
}

type Step struct {
	Name string
	Run  func(ctx context.Context, s *State) error
}

func notImplemented(context.Context, *State) error {
	return errNotImplemented
}

var Steps = []Step{
	{Name: "platform-check", Run: notImplemented},
	{Name: "select-pins", Run: notImplemented},
	{Name: "plan", Run: notImplemented},
	{Name: "lock", Run: notImplemented},
	{Name: "plan-again", Run: notImplemented},
	{Name: "deps", Run: notImplemented},
	{Name: "image", Run: notImplemented},
	{Name: "master-key", Run: notImplemented},
	{Name: "cert", Run: notImplemented},
	{Name: "sag-yaml", Run: notImplemented},
	{Name: "autostart", Run: notImplemented},
	{Name: "service-start", Run: notImplemented},
	{Name: "tailscale", Run: notImplemented},
	{Name: "agent-setup", Run: notImplemented},
	{Name: "print-endpoint", Run: notImplemented},
}
