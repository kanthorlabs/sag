package podman

import (
	"context"

	"github.com/kanthorlabs/sag/internal/podman"
	"github.com/kanthorlabs/sag/internal/secret"
)

type Backend struct {
	runner podman.Runner
}

func New(runner podman.Runner) *Backend {
	return &Backend{runner: runner}
}

func (b *Backend) Put(ctx context.Context, name string, value secret.Credential, labels map[string]string) error {
	panic("not implemented")
}

func (b *Backend) Load(ctx context.Context, name string) (secret.Credential, error) {
	panic("not implemented")
}

func (b *Backend) Delete(ctx context.Context, name string) error {
	panic("not implemented")
}

func (b *Backend) Metadata(ctx context.Context, name string) (secret.Metadata, error) {
	panic("not implemented")
}

func (b *Backend) List(ctx context.Context, selector map[string]string) ([]secret.Metadata, error) {
	panic("not implemented")
}
