package scrub

import "github.com/kanthorlabs/sag/internal/secret"

type Scrubber struct {
	secrets []secret.Credential
}

func New(secrets []secret.Credential) Scrubber {
	return Scrubber{secrets: secrets}
}

func (s Scrubber) String(text string) string {
	return text
}

func (s Scrubber) Error(err error) error {
	return err
}
