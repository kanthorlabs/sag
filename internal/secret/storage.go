package secret

import (
	"context"
	"errors"
	"time"
)

type Metadata struct {
	Name      string
	Labels    map[string]string
	CreatedAt time.Time
}

var (
	ErrExists      = errors.New("secret exists")
	ErrNotFound    = errors.New("secret not found")
	ErrUnavailable = errors.New("secret backend unavailable")
)

type SecureStorage interface {
	Put(ctx context.Context, name string, value Credential, labels map[string]string) error
	Load(ctx context.Context, name string) (Credential, error)
	Delete(ctx context.Context, name string) error
	Metadata(ctx context.Context, name string) (Metadata, error)
	List(ctx context.Context, selector map[string]string) ([]Metadata, error)
}
