package credential

import (
	"context"

	"github.com/kanthorlabs/sag/internal/deploy"
	"github.com/kanthorlabs/sag/internal/podman"
	"github.com/kanthorlabs/sag/internal/secret"
)

func revokeOneShot(ctx context.Context, name string, pm podman.Command, image string) error {
	p := deploy.DefaultFlags.Product
	masterKeySecret := p.Secret(secret.MasterKeyName, secret.MasterKeyName, "")
	stateVol := p.StateVolume("sag-state")

	args := []string{"run", "--rm"}
	args = append(args, p.SecurityArgs()...)
	args = append(args, "-v", stateVol)
	args = append(args, "--secret", masterKeySecret)
	args = append(args, image)
	args = append(args, "extension", "revoke", name)

	_, err := pm.Run(ctx, nil, args...)
	return err
}
