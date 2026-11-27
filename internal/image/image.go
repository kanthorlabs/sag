package image

import (
	"context"
	"errors"
	"strings"

	"github.com/kanthorlabs/sag/internal/podman"
	"github.com/kanthorlabs/sag/internal/stepline"
	"github.com/kanthorlabs/sag/internal/testhook"
)

func Ensure(ctx context.Context, pm podman.Command, ref string, out stepline.Writer) (changed bool, err error) {
	_, err = pm.Run(ctx, nil, "image", "exists", ref)
	if err == nil {
		out.Line(stepline.StatusOK, "image", ref)
		return false, nil
	}
	var runErr *podman.RunError
	if !errors.As(err, &runErr) {
		return false, err
	}
	if runErr.ExitCode != 1 {
		return false, err
	}

	pullArgs := []string{"pull"}
	if reg := testhook.Registry(); reg != "" && strings.HasPrefix(ref, reg+"/") {
		pullArgs = append(pullArgs, "--tls-verify=false")
	}
	pullArgs = append(pullArgs, ref)
	if _, err := pm.Run(ctx, nil, pullArgs...); err != nil {
		return false, err
	}
	_, devBuild := Reference()
	if devBuild {
		out.Line(stepline.StatusWarn, "image", "dev build; pulls the main tag")
	}
	out.Line(stepline.StatusInstall, "image", ref)
	return true, nil
}
