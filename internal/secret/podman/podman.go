package podman

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"strings"

	ipodman "github.com/kanthorlabs/sag/internal/podman"
	"github.com/kanthorlabs/sag/internal/secret"
)

type secretRunner interface {
	SecretCreate(ctx context.Context, name string, value io.Reader, labels map[string]string) error
	SecretInspect(ctx context.Context, name string) (ipodman.SecretInfo, error)
	SecretList(ctx context.Context) ([]string, error)
	SecretRemove(ctx context.Context, name string) error
}

type Backend struct {
	runner secretRunner
	Dir    string
	UID    int
}

func New(runner secretRunner) *Backend {
	return &Backend{runner: runner, Dir: secret.Dir, UID: secret.ContainerUID}
}

func (b *Backend) Put(ctx context.Context, name string, value secret.Credential, labels map[string]string) error {
	err := b.runner.SecretCreate(ctx, name, bytes.NewReader(value.Reveal()), labels)
	if err == nil {
		return nil
	}
	if isExistsError(err) {
		return secret.ErrExists
	}
	if isUnavailableError(err) {
		return secret.ErrUnavailable
	}
	return err
}

func (b *Backend) Load(ctx context.Context, name string) (secret.Credential, error) {
	creds, err := secret.Load(b.Dir, b.UID, []string{name})
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return secret.Credential{}, secret.ErrNotFound
		}
		return secret.Credential{}, err
	}
	return creds[name], nil
}

func (b *Backend) Delete(ctx context.Context, name string) error {
	err := b.runner.SecretRemove(ctx, name)
	if err == nil || isNotFoundError(err) {
		return nil
	}
	if isUnavailableError(err) {
		return secret.ErrUnavailable
	}
	return err
}

func (b *Backend) Metadata(ctx context.Context, name string) (secret.Metadata, error) {
	info, err := b.runner.SecretInspect(ctx, name)
	if err != nil {
		if isNotFoundError(err) || errors.Is(err, ipodman.ErrEmptyInspect) {
			return secret.Metadata{}, secret.ErrNotFound
		}
		if isUnavailableError(err) {
			return secret.Metadata{}, secret.ErrUnavailable
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
	names, err := b.runner.SecretList(ctx)
	if err != nil {
		if isUnavailableError(err) {
			return nil, secret.ErrUnavailable
		}
		return nil, err
	}
	var result []secret.Metadata
	for _, name := range names {
		meta, err := b.Metadata(ctx, name)
		if err != nil {
			if errors.Is(err, secret.ErrNotFound) {
				continue
			}
			return nil, err
		}
		if matchesSelector(meta.Labels, selector) {
			result = append(result, meta)
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
	var re *ipodman.RunError
	if !errors.As(err, &re) {
		return false
	}
	return strings.Contains(re.Stderr, "secret name in use") || strings.Contains(re.Stderr, "secret ID already in use")
}

func isNotFoundError(err error) bool {
	var re *ipodman.RunError
	if !errors.As(err, &re) {
		return false
	}
	return strings.Contains(re.Stderr, "no secret with name") || strings.Contains(re.Stderr, "no such secret")
}

func isUnavailableError(err error) bool {
	var re *ipodman.RunError
	if !errors.As(err, &re) {
		return false
	}
	if re.ExitCode == -1 {
		return true
	}
	return strings.Contains(re.Stderr, "unable to connect") || strings.Contains(re.Stderr, "connect: no such file")
}

var _ secret.SecureStorage = (*Backend)(nil)
