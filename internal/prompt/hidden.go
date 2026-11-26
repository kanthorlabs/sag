package prompt

import (
	"fmt"
	"os"

	"golang.org/x/term"

	"github.com/kanthorlabs/sag/internal/secret"
)

func ReadHidden(tty *os.File, label string) (secret.Credential, error) {
	fmt.Fprint(tty, label)
	value, err := term.ReadPassword(int(tty.Fd()))
	if err != nil {
		return secret.Credential{}, err
	}
	fmt.Fprintln(tty)
	return secret.NewCredential(label, value), nil
}
