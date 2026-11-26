package dxh

import "context"

func GroupSetupImage(ctx context.Context, r Runner) error {
	_, err := r.Output(ctx, "podman", "tag", "localhost/sag:devx-e2e", "docker.io/kanthorlabs/sag:main")
	return err
}
