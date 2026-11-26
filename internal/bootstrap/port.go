package bootstrap

import (
	"context"
	"fmt"

	"github.com/kanthorlabs/sag/internal/portcheck"
	"github.com/kanthorlabs/sag/internal/stepline"
)

func defaultPlanPort(
	check func(context.Context, string) (portcheck.Holder, error),
) func(context.Context, *State) error {
	return func(ctx context.Context, s *State) error {
		h, err := check(ctx, s.PodmanPath)
		if err != nil {
			return err
		}
		switch h.Kind {
		case portcheck.HolderNone:
			return nil
		case portcheck.HolderSAG:
			if s.PortHolder.Kind != portcheck.HolderSAG {
				s.Out.Line(stepline.StatusOK, "port-8443", "")
			}
			s.PortHolder = h
			return nil
		case portcheck.HolderContainer:
			s.Out.Line(stepline.StatusFail, "port", "8443 held by container "+h.Name)
			s.PortHolder = h
			return fmt.Errorf("port 8443 held by container %s", h.Name)
		case portcheck.HolderProcess:
			s.Out.Line(stepline.StatusFail, "port", "8443 held by "+h.Name)
			s.PortHolder = h
			return fmt.Errorf("port 8443 held by %s", h.Name)
		default:
			return nil
		}
	}
}
