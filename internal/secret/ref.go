package secret

import (
	"errors"
	"strings"
)

const (
	BackendPodman = "secret"
	BackendAWSSM  = "aws-sm"
)

type Ref struct {
	Backend string
	Name    string
}

func ParseRef(s string) (Ref, error) {
	for _, prefix := range []string{BackendPodman + ":", BackendAWSSM + ":"} {
		if name, ok := strings.CutPrefix(s, prefix); ok {
			backend := strings.TrimSuffix(prefix, ":")
			return Ref{Backend: backend, Name: name}, nil
		}
	}
	return Ref{}, errors.New("secret: unrecognized reference " + s)
}
