package prompt

import (
	"errors"
	"os"

	_ "golang.org/x/term"

	"github.com/kanthorlabs/sag/internal/secret"
)

func ReadHidden(tty *os.File, label string) (secret.Credential, error) {
	return secret.Credential{}, errors.New("not implemented")
}
