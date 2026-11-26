package image

import (
	"context"

	"github.com/kanthorlabs/sag/internal/deploy"
	"github.com/kanthorlabs/sag/internal/podman"
	"github.com/kanthorlabs/sag/internal/stepline"
)

const (
	devReference = "docker.io/kanthorlabs/sag:main"
)

func productReference() string {
	if deploy.ImageDigest != "" {
		return "docker.io/kanthorlabs/sag@" + deploy.ImageDigest
	}
	return devReference
}

func Ensure(ctx context.Context, pm podman.Command, ref string, out stepline.Writer) (changed bool, err error) {
	_, err = pm.Run(ctx, nil, "image", "exists", ref)
	if err == nil {
		out.Line(stepline.StatusOK, "image", ref)
		return false, nil
	}
	if _, ok := err.(*podman.RunError); !ok {
		return false, err
	}

	if _, err := pm.Run(ctx, nil, "pull", ref); err != nil {
		return false, err
	}
	if deploy.ImageDigest == "" {
		out.Line(stepline.StatusWarn, "image", "dev build; pulls the main tag")
	}
	out.Line(stepline.StatusInstall, "image", ref)
	return true, nil
}
