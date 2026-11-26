package credential

import (
	"context"
	"errors"
)

type CreateOptions struct{}

func Create(ctx context.Context, opts CreateOptions) error {
	return errors.New("not implemented")
}
