package secret

import (
	"errors"
	"regexp"
	"strings"
)

const (
	BackendPodman = "secret"
	BackendAWSSM  = "aws-sm"
)

var awsNamePattern = regexp.MustCompile(`^[A-Za-z0-9/_+=.@-]{1,512}$`)

type Ref struct {
	Backend string
	Name    string
}

func ParseRef(s string) (Ref, error) {
	for _, prefix := range []string{BackendPodman + ":", BackendAWSSM + ":"} {
		if name, ok := strings.CutPrefix(s, prefix); ok {
			backend := strings.TrimSuffix(prefix, ":")
			if backend == BackendAWSSM && !awsNamePattern.MatchString(name) {
				return Ref{}, errors.New("secret: invalid aws-sm name " + name)
			}
			return Ref{Backend: backend, Name: name}, nil
		}
	}
	return Ref{}, errors.New("secret: unrecognized reference " + s)
}
