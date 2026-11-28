package awssm

import (
	"context"
	"errors"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	awssm "github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/kanthorlabs/sag/internal/secret"
)

type Backend struct {
	client *awssm.Client
}

func New(cfg aws.Config) *Backend {
	return &Backend{client: awssm.NewFromConfig(cfg)}
}

func NewFromRegion(ctx context.Context, region string) (*Backend, error) {
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	if err != nil {
		return nil, err
	}
	return New(cfg), nil
}

func (b *Backend) Put(_ context.Context, _ string, _ secret.Credential, _ map[string]string) error {
	return errors.New("awssm: not implemented")
}

func (b *Backend) Load(_ context.Context, _ string) (secret.Credential, error) {
	return secret.Credential{}, errors.New("awssm: not implemented")
}

func (b *Backend) Delete(_ context.Context, _ string) error {
	return errors.New("awssm: not implemented")
}

func (b *Backend) Metadata(_ context.Context, _ string) (secret.Metadata, error) {
	return secret.Metadata{}, errors.New("awssm: not implemented")
}

func (b *Backend) List(_ context.Context, _ map[string]string) ([]secret.Metadata, error) {
	return nil, errors.New("awssm: not implemented")
}
