package credential

import (
	"context"
	"errors"

	"github.com/kanthorlabs/sag/internal/config"
	"github.com/kanthorlabs/sag/internal/secret"
	"github.com/kanthorlabs/sag/internal/secret/awssm"
)

func BackendFor(cfg *config.Config, name string) (secret.SecureStorage, error) {
	switch name {
	case secret.BackendAWSSM:
		if cfg.Storage.AWS.Region == "" {
			return nil, errors.New("credential: backend aws-sm needs storage.aws in sag.yaml")
		}
		return awssm.NewFromRegion(context.Background(), cfg.Storage.AWS.Region)
	default:
		return nil, errors.New("credential: unknown backend " + name)
	}
}
