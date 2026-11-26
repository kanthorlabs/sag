package podman

import (
	"bytes"
	"context"
	"io"
	"strings"

	ipodman "github.com/kanthorlabs/sag/internal/podman"
	"github.com/kanthorlabs/sag/internal/secret"
)

type secretRunner interface {
	SecretCreate(ctx context.Context, name string, value io.Reader, labels map[string]string) error
	SecretInspect(ctx context.Context, name string) (ipodman.SecretInfo, error)
	SecretList(ctx context.Context) ([]ipodman.SecretInfo, error)
	SecretRemove(ctx context.Context, name string) error
}

type Backend struct {
	runner secretRunner
}

func New(runner secretRunner) *Backend {
	return &Backend{runner: runner}
}

func (b *Backend) Put(ctx context.Context, name string, value secret.Credential, labels map[string]string) error {
	err := b.runner.SecretCreate(ctx, name, bytes.NewReader(value.Reveal()), labels)
	if err == nil {
		return nil
	}
	if isExistsError(err) {
		return secret.ErrExists
	}
	return err
}

func (b *Backend) Load(ctx context.Context, name string) (secret.Credential, error) {
	creds, err := secret.Load(secret.Dir, secret.ContainerUID, []string{name})
	if err != nil {
		return secret.Credential{}, err
	}
	cred, ok := creds[name]
	if !ok {
		return secret.Credential{}, secret.ErrNotFound
	}
	return secret.NewCredential(name, cred.Reveal()), nil
}

func (b *Backend) Delete(ctx context.Context, name string) error {
	err := b.runner.SecretRemove(ctx, name)
	if err == nil || isNotFoundError(err) {
		return nil
	}
	return err
}

func (b *Backend) Metadata(ctx context.Context, name string) (secret.Metadata, error) {
	info, err := b.runner.SecretInspect(ctx, name)
	if err != nil {
		if isNotFoundError(err) {
			return secret.Metadata{}, secret.ErrNotFound
		}
		return secret.Metadata{}, err
	}
	return secret.Metadata{
		Name:      info.Spec.Name,
		Labels:    info.Spec.Labels,
		CreatedAt: info.CreatedAt,
	}, nil
}

func (b *Backend) List(ctx context.Context, selector map[string]string) ([]secret.Metadata, error) {
	all, err := b.runner.SecretList(ctx)
	if err != nil {
		return nil, err
	}
	var result []secret.Metadata
	for _, info := range all {
		if matchesSelector(info.Spec.Labels, selector) {
			result = append(result, secret.Metadata{
				Name:      info.Spec.Name,
				Labels:    info.Spec.Labels,
				CreatedAt: info.CreatedAt,
			})
		}
	}
	return result, nil
}

func matchesSelector(labels, selector map[string]string) bool {
	for k, v := range selector {
		if labels[k] != v {
			return false
		}
	}
	return true
}

func isExistsError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "secret already exists") || strings.Contains(msg, "already exist")
}

func isNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "no secret with name") || strings.Contains(msg, "no such secret") || strings.Contains(msg, "not found")
}

var _ secret.SecureStorage = (*Backend)(nil)
