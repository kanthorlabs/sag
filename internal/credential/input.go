package credential

import (
	"bytes"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"regexp"

	"github.com/kanthorlabs/sag/internal/secret"
)

const maxStdinBytes = 64 * 1024

var (
	namePattern  = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)
	agentPattern = namePattern
)

func validateName(name string) error {
	if !namePattern.MatchString(name) {
		return fmt.Errorf("invalid name %q; must match ^[a-z][a-z0-9-]{0,62}$", name)
	}
	return nil
}

func validateAgents(agents []string) error {
	if len(agents) == 0 {
		return errors.New("at least one --agent is required")
	}
	for _, a := range agents {
		if !agentPattern.MatchString(a) {
			return fmt.Errorf("invalid agent %q; must match ^[a-z][a-z0-9-]{0,62}$", a)
		}
	}
	return nil
}

func validateDescription(desc string) error {
	if len([]rune(desc)) > 500 {
		return errors.New("--description: at most 500 characters")
	}
	return nil
}

func validateTLSCAPem(pem []byte) error {
	if len(pem) > 0 && !hasCertificate(pem) {
		return errors.New("--tls-ca: no valid PEM certificate found")
	}
	return nil
}

func hasCertificate(data []byte) bool {
	for len(data) > 0 {
		var block *pem.Block
		block, data = pem.Decode(data)
		if block == nil {
			break
		}
		if block.Type == "CERTIFICATE" {
			return true
		}
	}
	return false
}

func readStdin(r io.Reader, isTerminal bool) (secret.Credential, error) {
	if isTerminal {
		return secret.Credential{}, errors.New("--stdin: standard input is a terminal; use a pipe or omit --stdin")
	}
	buf := make([]byte, maxStdinBytes+1)
	n, err := io.ReadFull(r, buf)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return secret.Credential{}, err
	}
	if n > maxStdinBytes {
		return secret.Credential{}, errors.New("--stdin: input exceeds 64 KiB limit")
	}
	raw := buf[:n]
	if bytes.ContainsRune(raw, 0) {
		return secret.Credential{}, errors.New("--stdin: NUL byte in input")
	}
	raw = bytes.TrimSuffix(raw, []byte("\r\n"))
	raw = bytes.TrimSuffix(raw, []byte("\n"))
	if bytes.ContainsRune(raw, '\n') {
		return secret.Credential{}, errors.New("--stdin: inner line break in input")
	}
	return secret.NewCredential("stdin", raw), nil
}
