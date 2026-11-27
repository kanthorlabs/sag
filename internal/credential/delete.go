package credential

import (
	"context"
	"errors"
	"io"
	"os"

	"github.com/kanthorlabs/sag/internal/podman"
	"github.com/kanthorlabs/sag/internal/secret"
)

type DeleteOptions struct {
	CfgDir       string
	SAGYaml      string
	Backend      secret.SecureStorage
	Podman       podman.Runner
	ServiceStart func(ctx context.Context, out io.Writer) error
	Out          io.Writer
	Err          io.Writer
}

func Delete(ctx context.Context, name string, tty *os.File, opts DeleteOptions) error {
	return errors.New("not implemented")
}
