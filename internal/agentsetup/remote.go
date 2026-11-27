package agentsetup

import (
	"context"
	"io"

	"github.com/kanthorlabs/sag/internal/secret"
)

func ParseURL(url string) (Mode, error) {
	return ModeLocal, errNotImplemented
}

func ReadJWT(tty io.ReadWriter, sub string) (secret.Credential, error) {
	return secret.Credential{}, errNotImplemented
}

func planRemote(ctx context.Context, opts Options) error {
	return errNotImplemented
}

func remoteJWT(ctx context.Context, opts Options) (secret.Credential, error) {
	return secret.Credential{}, errNotImplemented
}
