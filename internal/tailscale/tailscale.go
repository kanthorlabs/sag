package tailscale

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"os/exec"
	"strings"

	"github.com/kanthorlabs/sag/internal/stepline"
)

const (
	SkipCommandAbsent  = "command absent"
	SkipDown           = "down"
	SkipAddressInvalid = "address invalid"
	SkipOperatorNotSet = "operator not set"

	sagTCPForward = "127.0.0.1:8443"
	sagPort       = "8443"
)

var ErrOperatorSkip = errors.New("tailscale: operator not set; skip")

var cgnatNet *net.IPNet

func init() {
	_, cgnatNet, _ = net.ParseCIDR("100.64.0.0/10")
}

type State struct {
	Address      string
	SkipReason   string
	HasSAGRule   bool
	OtherTarget  bool
	FunnelActive bool
	Planned      bool
}

type Command interface {
	Output(ctx context.Context, args ...string) ([]byte, error)
}

type osCommand struct {
	path string
}

func (c osCommand) Output(ctx context.Context, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, c.path, args...).Output()
}

func LookupCommand() (Command, bool) {
	path, err := exec.LookPath("tailscale")
	if err != nil {
		return nil, false
	}
	return osCommand{path: path}, true
}

func ValidateCGNATAddress(addr string) bool {
	ip := net.ParseIP(addr)
	return ip != nil && cgnatNet.Contains(ip)
}

type serveStatus struct {
	TCP         map[string]json.RawMessage `json:"TCP"`
	AllowFunnel map[string]bool            `json:"AllowFunnel"`
}

func Detect(ctx context.Context, cmd Command) (State, error) {
	out, err := cmd.Output(ctx, "ip", "-4")
	if err != nil {
		return State{SkipReason: SkipDown}, nil
	}

	addr := strings.TrimSpace(strings.SplitN(string(out), "\n", 2)[0])
	if !ValidateCGNATAddress(addr) {
		return State{SkipReason: SkipAddressInvalid}, nil
	}

	statusOut, err := cmd.Output(ctx, "serve", "status", "--json")
	if err != nil {
		return State{SkipReason: SkipDown}, nil
	}

	var status serveStatus
	if err := json.Unmarshal(statusOut, &status); err != nil {
		return State{SkipReason: SkipDown}, nil
	}

	hasRule := false
	otherTarget := false

	if rawEntry, ok := status.TCP[sagPort]; ok {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(rawEntry, &fields); err != nil || len(fields) != 1 {
			otherTarget = true
		} else if _, hasFwd := fields["TCPForward"]; !hasFwd {
			otherTarget = true
		} else {
			var target string
			if err := json.Unmarshal(fields["TCPForward"], &target); err != nil || target != sagTCPForward {
				otherTarget = true
			} else {
				hasRule = true
			}
		}
	}

	funnelActive := false
	for key, val := range status.AllowFunnel {
		if strings.HasSuffix(key, ":"+sagPort) && val {
			funnelActive = true
			break
		}
	}

	if funnelActive {
		hasRule = false
	}

	return State{
		Address:      addr,
		HasSAGRule:   hasRule,
		OtherTarget:  otherTarget,
		FunnelActive: funnelActive,
	}, nil
}

func Apply(ctx context.Context, state State, cmd Command, out stepline.Writer) error {
	if state.HasSAGRule {
		out.Line(stepline.StatusOK, "tailscale-serve", "")
		return nil
	}
	if _, err := cmd.Output(ctx, "serve", "--bg", "--tcp", sagPort, "tcp://"+sagTCPForward); err != nil {
		return err
	}
	out.Line(stepline.StatusInstall, "tailscale-serve", "")
	return nil
}
