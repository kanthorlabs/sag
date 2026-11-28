package awssm

import (
	"context"
	"errors"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	awssm "github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager/types"
	"github.com/kanthorlabs/sag/internal/secret"
)

const callTimeout = 10 * time.Second

type smClient interface {
	CreateSecret(ctx context.Context, params *awssm.CreateSecretInput, optFns ...func(*awssm.Options)) (*awssm.CreateSecretOutput, error)
	GetSecretValue(ctx context.Context, params *awssm.GetSecretValueInput, optFns ...func(*awssm.Options)) (*awssm.GetSecretValueOutput, error)
	DeleteSecret(ctx context.Context, params *awssm.DeleteSecretInput, optFns ...func(*awssm.Options)) (*awssm.DeleteSecretOutput, error)
	DescribeSecret(ctx context.Context, params *awssm.DescribeSecretInput, optFns ...func(*awssm.Options)) (*awssm.DescribeSecretOutput, error)
	ListSecrets(ctx context.Context, params *awssm.ListSecretsInput, optFns ...func(*awssm.Options)) (*awssm.ListSecretsOutput, error)
}

type Backend struct {
	client smClient
}

func New(cfg aws.Config) *Backend {
	return &Backend{client: awssm.NewFromConfig(cfg)}
}

func NewFromRegion(ctx context.Context, region string) (*Backend, error) {
	cfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(region))
	if err != nil {
		return nil, err
	}
	return New(cfg), nil
}

func NewDaemon(ctx context.Context, region, credentialsName string) (*Backend, error) {
	credPath := "/run/secrets/" + credentialsName
	cfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion(region),
		awsconfig.WithSharedCredentialsFiles([]string{credPath}),
	)
	if err != nil {
		return nil, err
	}
	return New(cfg), nil
}

func (b *Backend) Put(ctx context.Context, name string, value secret.Credential, labels map[string]string) error {
	ctx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()

	tags := labelsToTags(labels)
	str := string(value.Reveal())
	_, err := b.client.CreateSecret(ctx, &awssm.CreateSecretInput{
		Name:         aws.String(name),
		SecretString: aws.String(str),
		Tags:         tags,
	})
	if err != nil {
		var exists *types.ResourceExistsException
		if errors.As(err, &exists) {
			return secret.ErrExists
		}
		return mapErr(err)
	}
	return nil
}

func (b *Backend) Load(ctx context.Context, name string) (secret.Credential, error) {
	ctx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()

	out, err := b.client.GetSecretValue(ctx, &awssm.GetSecretValueInput{
		SecretId: aws.String(name),
	})
	if err != nil {
		var notFound *types.ResourceNotFoundException
		if errors.As(err, &notFound) {
			return secret.Credential{}, secret.ErrNotFound
		}
		return secret.Credential{}, mapErr(err)
	}
	if out.SecretString == nil {
		return secret.Credential{}, secret.ErrNotFound
	}
	return secret.NewCredential(name, []byte(*out.SecretString)), nil
}

func (b *Backend) Delete(ctx context.Context, name string) error {
	ctx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()

	force := true
	_, err := b.client.DeleteSecret(ctx, &awssm.DeleteSecretInput{
		SecretId:                   aws.String(name),
		ForceDeleteWithoutRecovery: &force,
	})
	if err != nil {
		var notFound *types.ResourceNotFoundException
		if errors.As(err, &notFound) {
			return nil
		}
		return mapErr(err)
	}
	return nil
}

func (b *Backend) Metadata(ctx context.Context, name string) (secret.Metadata, error) {
	ctx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()

	out, err := b.client.DescribeSecret(ctx, &awssm.DescribeSecretInput{
		SecretId: aws.String(name),
	})
	if err != nil {
		var notFound *types.ResourceNotFoundException
		if errors.As(err, &notFound) {
			return secret.Metadata{}, secret.ErrNotFound
		}
		return secret.Metadata{}, mapErr(err)
	}
	meta := secret.Metadata{
		Labels: tagsToLabels(out.Tags),
	}
	if out.Name != nil {
		meta.Name = *out.Name
	}
	if out.CreatedDate != nil {
		meta.CreatedAt = *out.CreatedDate
	}
	return meta, nil
}

func (b *Backend) List(ctx context.Context, selector map[string]string) ([]secret.Metadata, error) {
	ctx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()

	var tagKeys []string
	for k := range selector {
		tagKeys = append(tagKeys, k)
	}

	filters := []types.Filter{
		{
			Key:    types.FilterNameStringTypeTagKey,
			Values: tagKeys,
		},
	}

	var result []secret.Metadata
	var token *string
	for {
		out, err := b.client.ListSecrets(ctx, &awssm.ListSecretsInput{
			Filters:   filters,
			NextToken: token,
		})
		if err != nil {
			return nil, mapErr(err)
		}
		for _, entry := range out.SecretList {
			labels := tagsToLabels(entry.Tags)
			if !matchesSelector(labels, selector) {
				continue
			}
			meta := secret.Metadata{
				Labels: labels,
			}
			if entry.Name != nil {
				meta.Name = *entry.Name
			}
			if entry.CreatedDate != nil {
				meta.CreatedAt = *entry.CreatedDate
			}
			result = append(result, meta)
		}
		if aws.ToString(out.NextToken) == "" {
			break
		}
		token = out.NextToken
	}
	return result, nil
}

func labelsToTags(labels map[string]string) []types.Tag {
	tags := make([]types.Tag, 0, len(labels))
	for k, v := range labels {
		k, v := k, v
		tags = append(tags, types.Tag{Key: &k, Value: &v})
	}
	return tags
}

func tagsToLabels(tags []types.Tag) map[string]string {
	labels := make(map[string]string, len(tags))
	for _, t := range tags {
		if t.Key != nil && t.Value != nil {
			labels[*t.Key] = *t.Value
		}
	}
	return labels
}

func matchesSelector(labels, selector map[string]string) bool {
	for k, v := range selector {
		if labels[k] != v {
			return false
		}
	}
	return true
}

func mapErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.DeadlineExceeded) || isTimeoutErr(err) {
		return secret.ErrUnavailable
	}
	var svcErr interface{ ErrorCode() string }
	if errors.As(err, &svcErr) {
		return secret.ErrUnavailable
	}
	return secret.ErrUnavailable
}

func isTimeoutErr(err error) bool {
	type timeoutError interface {
		Timeout() bool
	}
	var te timeoutError
	return errors.As(err, &te) && te.Timeout()
}

var _ secret.SecureStorage = (*Backend)(nil)
