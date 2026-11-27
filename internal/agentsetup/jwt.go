package agentsetup

import (
	"context"

	"github.com/kanthorlabs/sag/internal/secret"
)

func issueJWT(ctx context.Context, opts Options) (secret.Credential, error) {
	return secret.Credential{}, errNotImplemented
}
