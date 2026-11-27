package bootstrap

import (
	"context"

	"github.com/kanthorlabs/sag/internal/agentsetup"
	"github.com/kanthorlabs/sag/internal/deps"
	"github.com/kanthorlabs/sag/internal/hoststate"
	"github.com/kanthorlabs/sag/internal/pins"
	"github.com/kanthorlabs/sag/internal/portcheck"
	"github.com/kanthorlabs/sag/internal/prompt"
	"github.com/kanthorlabs/sag/internal/stepline"
	"github.com/kanthorlabs/sag/internal/tailscale"
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
	Flags             Flags
	Paths             hoststate.Paths
	Answers           hoststate.Answers
	Lock              *hoststate.Lock
	Asker             prompt.Asker
	Out               stepline.Writer
	PodmanPath        string
	SAGPath           string
	UID               string
	Home              string
	CfgCreated        bool
	Changed           bool
	Pins              []pins.Item
	DepPlan           deps.Plan
	TailscaleState    tailscale.State
	PortHolder        portcheck.Holder
	AgentSetupAnswers map[string]agentsetup.ResolvedAnswers
}

type Step struct {
	Name string
	Run  func(ctx context.Context, s *State) error
}
