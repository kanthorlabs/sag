package podman

import (
	"context"
	"io"
)

type Runner struct {
	Path string
}

func (r Runner) Run(ctx context.Context, stdin io.Reader, args ...string) ([]byte, error) {
	panic("not implemented")
}

func (r Runner) SecretCreate(ctx context.Context, name string, value io.Reader, labels map[string]string) error {
	panic("not implemented")
}

func (r Runner) SecretInspect(ctx context.Context, name string) (SecretInfo, error) {
	panic("not implemented")
}

func (r Runner) SecretList(ctx context.Context) ([]SecretInfo, error) {
	panic("not implemented")
}

func (r Runner) SecretRemove(ctx context.Context, name string) error {
	panic("not implemented")
}

func (r Runner) ContainerInspect(ctx context.Context, name string) (ContainerInfo, error) {
	panic("not implemented")
}
