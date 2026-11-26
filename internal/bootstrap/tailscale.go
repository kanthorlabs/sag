package bootstrap

import (
	"context"
	"errors"

	"github.com/kanthorlabs/sag/internal/stepline"
	"github.com/kanthorlabs/sag/internal/tailscale"
)

func (d Deps) stepTailscale(ctx context.Context, s *State) error {
	if !s.TailscaleState.Planned {
		return nil
	}
	if s.TailscaleState.SkipReason != "" {
		s.Out.Line(stepline.StatusSkip, "tailscale", s.TailscaleState.SkipReason+"; loopback only")
		return nil
	}
	cmd, found := tailscale.LookupCommand()
	if !found {
		return errors.New("tailscale: command not found at step 13")
	}
	return tailscale.Apply(ctx, s.TailscaleState, cmd, s.Out)
}

func defaultPlanTailscale(ctx context.Context, s *State) error {
	cmd, found := tailscale.LookupCommand()
	if !found {
		s.TailscaleState = tailscale.State{Planned: true, SkipReason: tailscale.SkipCommandAbsent}
		return nil
	}
	state, err := tailscale.Detect(ctx, cmd)
	if err != nil {
		return err
	}
	if state.SkipReason == tailscale.SkipDown {
		s.TailscaleState.SkipReason = tailscale.SkipDown
		s.TailscaleState.Planned = true
		return nil
	}
	if state.SkipReason != "" {
		s.TailscaleState = tailscale.State{Planned: true, SkipReason: state.SkipReason}
		return nil
	}
	if state.OtherTarget {
		s.Out.Line(stepline.StatusFail, "tailscale", "TCP 8443 serves another target")
		return errors.New("tailscale: TCP 8443 serves another target")
	}
	if state.FunnelActive {
		s.Out.Line(stepline.StatusFail, "tailscale", "funnel is on for port 8443")
		return errors.New("tailscale: funnel is on for port 8443")
	}
	s.TailscaleState = state
	s.TailscaleState.Planned = true
	return nil
}
