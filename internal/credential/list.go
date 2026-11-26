package credential

import (
	"context"
	"errors"
	"time"
)

type SecretEntry struct {
	Name      string     `json:"name"`
	Backend   string     `json:"backend"`
	Role      string     `json:"role"`
	Exists    *bool      `json:"exists"`
	Owned     bool       `json:"owned"`
	CreatedAt *time.Time `json:"created_at"`
	Mounted   *bool      `json:"mounted,omitempty"`
}

type Entry struct {
	Name             string        `json:"name"`
	Kind             string        `json:"kind"`
	Description      string        `json:"description"`
	TLS              string        `json:"tls"`
	Agents           []string      `json:"agents"`
	Secrets          []SecretEntry `json:"secrets"`
	ContainerRunning *bool         `json:"container_running,omitempty"`
}

type ListOptions struct{}

type InspectOptions struct{}

func List(ctx context.Context, opts ListOptions) ([]Entry, error) {
	return nil, errors.New("not implemented")
}

func Inspect(ctx context.Context, name string, opts InspectOptions) (Entry, error) {
	return Entry{}, errors.New("not implemented")
}
