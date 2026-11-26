package tailscale

import "context"

type State struct {
	Address    string
	SkipReason string
	HasSAGRule bool
}

func Detect(_ context.Context) (State, error) {
	return State{}, nil
}

func Apply(_ context.Context, _ State) error {
	return nil
}
