package bootstrap

import (
	"context"

	"github.com/kanthorlabs/sag/internal/hoststate"
	"github.com/kanthorlabs/sag/internal/prompt"
	"github.com/kanthorlabs/sag/internal/stepline"
)

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
