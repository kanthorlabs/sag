package service

import (
	"context"
	"fmt"

	"github.com/kanthorlabs/sag/internal/podman"
	"github.com/kanthorlabs/sag/internal/portcheck"
	"github.com/kanthorlabs/sag/internal/stepline"
)

func checkPort(ctx context.Context, pm podman.Command, out stepline.Writer) error {
	var path string
	if r, ok := pm.(podman.Runner); ok {
		path = r.Path
	}
	h, err := portcheck.Check(ctx, path)
	if err != nil {
		return err
	}
	switch h.Kind {
	case portcheck.HolderNone, portcheck.HolderSAG:
		return nil
	case portcheck.HolderContainer:
		out.Line(stepline.StatusFail, "port", "8443 held by container "+h.Name)
		return fmt.Errorf("port 8443 held by container %s", h.Name)
	case portcheck.HolderProcess:
		out.Line(stepline.StatusFail, "port", "8443 held by "+h.Name)
		return fmt.Errorf("port 8443 held by %s", h.Name)
	default:
		return nil
	}
}
