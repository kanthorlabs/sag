package credential

import (
	"errors"

	"github.com/kanthorlabs/sag/internal/config"
	"github.com/kanthorlabs/sag/internal/secret"
)

func BackendFor(_ *config.Config, _ string) (secret.SecureStorage, error) {
	return nil, errors.New("credential: not implemented")
}
