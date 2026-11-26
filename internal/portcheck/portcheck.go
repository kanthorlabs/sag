package portcheck

import "context"

type HolderKind int

const (
	HolderNone HolderKind = iota
	HolderSAG
	HolderContainer
	HolderProcess
)

type Holder struct {
	Kind HolderKind
	Name string
}

func Check(_ context.Context) (Holder, error) {
	return Holder{}, nil
}
