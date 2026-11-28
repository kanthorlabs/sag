package awssm

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awssm "github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager/types"
	"github.com/kanthorlabs/sag/internal/secret"
)

type fakeClient struct {
	createIn  *awssm.CreateSecretInput
	createOut *awssm.CreateSecretOutput
	createErr error

	getOut *awssm.GetSecretValueOutput
	getErr error

	deleteIn  *awssm.DeleteSecretInput
	deleteOut *awssm.DeleteSecretOutput
	deleteErr error

	describeOut *awssm.DescribeSecretOutput
	describeErr error

	listOut   *awssm.ListSecretsOutput
	listPages map[string]*awssm.ListSecretsOutput
	listIn    *awssm.ListSecretsInput
	listErr   error

	callDelay time.Duration
}

func (f *fakeClient) CreateSecret(ctx context.Context, params *awssm.CreateSecretInput, _ ...func(*awssm.Options)) (*awssm.CreateSecretOutput, error) {
	f.createIn = params
	if f.callDelay > 0 {
		select {
		case <-time.After(f.callDelay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if f.createErr != nil {
		return nil, f.createErr
	}
	if f.createOut == nil {
		return &awssm.CreateSecretOutput{}, nil
	}
	return f.createOut, nil
}

func (f *fakeClient) GetSecretValue(ctx context.Context, _ *awssm.GetSecretValueInput, _ ...func(*awssm.Options)) (*awssm.GetSecretValueOutput, error) {
	if f.callDelay > 0 {
		select {
		case <-time.After(f.callDelay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if f.getErr != nil {
		return nil, f.getErr
	}
	if f.getOut == nil {
		return &awssm.GetSecretValueOutput{}, nil
	}
	return f.getOut, nil
}

func (f *fakeClient) DeleteSecret(ctx context.Context, params *awssm.DeleteSecretInput, _ ...func(*awssm.Options)) (*awssm.DeleteSecretOutput, error) {
	f.deleteIn = params
	if f.callDelay > 0 {
		select {
		case <-time.After(f.callDelay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if f.deleteErr != nil {
		return nil, f.deleteErr
	}
	if f.deleteOut == nil {
		return &awssm.DeleteSecretOutput{}, nil
	}
	return f.deleteOut, nil
}

func (f *fakeClient) DescribeSecret(ctx context.Context, _ *awssm.DescribeSecretInput, _ ...func(*awssm.Options)) (*awssm.DescribeSecretOutput, error) {
	if f.callDelay > 0 {
		select {
		case <-time.After(f.callDelay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if f.describeErr != nil {
		return nil, f.describeErr
	}
	if f.describeOut == nil {
		return &awssm.DescribeSecretOutput{}, nil
	}
	return f.describeOut, nil
}

func (f *fakeClient) ListSecrets(ctx context.Context, params *awssm.ListSecretsInput, _ ...func(*awssm.Options)) (*awssm.ListSecretsOutput, error) {
	f.listIn = params
	if f.callDelay > 0 {
		select {
		case <-time.After(f.callDelay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if f.listErr != nil {
		return nil, f.listErr
	}
	if f.listPages != nil {
		page, ok := f.listPages[aws.ToString(params.NextToken)]
		if !ok {
			return nil, fmt.Errorf("unknown page token %q", aws.ToString(params.NextToken))
		}
		return page, nil
	}
	if f.listOut == nil {
		return &awssm.ListSecretsOutput{}, nil
	}
	return f.listOut, nil
}

func newBackend(c smClient) *Backend {
	return &Backend{client: c}
}

func strPtr(s string) *string { return &s }

func TestPutTagsExactlyLabels(t *testing.T) {
	fc := &fakeClient{}
	b := newBackend(fc)

	labels := map[string]string{
		"sag.resource": "myres",
		"sag.kind":     "postgres",
		"sag.role":     "connection",
	}
	cred := secret.NewCredential("myname", []byte("myvalue"))
	err := b.Put(context.Background(), "sag/myres/conn", cred, labels)
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if fc.createIn == nil {
		t.Fatal("CreateSecret was not called")
	}
	if len(fc.createIn.Tags) != len(labels) {
		t.Fatalf("got %d tags, want %d", len(fc.createIn.Tags), len(labels))
	}
	got := map[string]string{}
	for _, tag := range fc.createIn.Tags {
		if tag.Key == nil || tag.Value == nil {
			t.Fatal("tag with nil key or value")
		}
		got[*tag.Key] = *tag.Value
	}
	for k, v := range labels {
		if got[k] != v {
			t.Errorf("tag %q = %q, want %q", k, got[k], v)
		}
	}
}

func TestPutExists(t *testing.T) {
	fc := &fakeClient{
		createErr: &types.ResourceExistsException{Message: strPtr("already exists")},
	}
	b := newBackend(fc)
	cred := secret.NewCredential("n", []byte("v"))
	err := b.Put(context.Background(), "sag/x/conn", cred, nil)
	if !errors.Is(err, secret.ErrExists) {
		t.Fatalf("got %v, want ErrExists", err)
	}
}

func TestDeleteForce(t *testing.T) {
	fc := &fakeClient{}
	b := newBackend(fc)
	err := b.Delete(context.Background(), "sag/x/conn")
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if fc.deleteIn == nil {
		t.Fatal("DeleteSecret was not called")
	}
	if fc.deleteIn.ForceDeleteWithoutRecovery == nil || !*fc.deleteIn.ForceDeleteWithoutRecovery {
		t.Error("ForceDeleteWithoutRecovery must be true")
	}
}

func TestDeleteAbsentNil(t *testing.T) {
	fc := &fakeClient{
		deleteErr: &types.ResourceNotFoundException{Message: strPtr("not found")},
	}
	b := newBackend(fc)
	err := b.Delete(context.Background(), "sag/absent/conn")
	if err != nil {
		t.Fatalf("Delete absent: got %v, want nil", err)
	}
}

func TestListTagFilter(t *testing.T) {
	name := "sag/myres/conn"
	k1, v1 := "sag.resource", "myres"
	k2, v2 := "sag.kind", "postgres"
	fc := &fakeClient{
		listOut: &awssm.ListSecretsOutput{
			SecretList: []types.SecretListEntry{
				{
					Name: &name,
					Tags: []types.Tag{
						{Key: &k1, Value: &v1},
						{Key: &k2, Value: &v2},
					},
				},
			},
		},
	}
	b := newBackend(fc)
	selector := map[string]string{k1: v1, k2: v2}
	metas, err := b.List(context.Background(), selector)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(metas) != 1 {
		t.Fatalf("got %d entries, want 1", len(metas))
	}
	if metas[0].Name != name {
		t.Errorf("got Name=%q, want %q", metas[0].Name, name)
	}
	if fc.listIn == nil {
		t.Fatal("ListSecrets was not called")
	}
	var tagKeyFilter *types.Filter
	for i := range fc.listIn.Filters {
		if fc.listIn.Filters[i].Key == types.FilterNameStringTypeTagKey {
			tagKeyFilter = &fc.listIn.Filters[i]
		}
	}
	if tagKeyFilter == nil {
		t.Fatal("no tag-key filter sent to ListSecrets")
	}
}

func TestListAllPages(t *testing.T) {
	entry := func(name, res string) types.SecretListEntry {
		k := "sag.resource"
		return types.SecretListEntry{Name: aws.String(name), Tags: []types.Tag{{Key: &k, Value: aws.String(res)}}}
	}
	fc := &fakeClient{listPages: map[string]*awssm.ListSecretsOutput{
		"": {
			SecretList: []types.SecretListEntry{entry("sag/other/conn", "other"), entry("sag/myres/conn", "myres")},
			NextToken:  aws.String("p2"),
		},
		"p2": {
			SecretList: []types.SecretListEntry{entry("sag/myres/ca", "myres")},
		},
	}}
	metas, err := newBackend(fc).List(context.Background(), map[string]string{"sag.resource": "myres"})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	var names []string
	for _, m := range metas {
		names = append(names, m.Name)
	}
	if !slices.Equal(names, []string{"sag/myres/conn", "sag/myres/ca"}) {
		t.Fatalf("names = %v; want both pages of myres", names)
	}
}

func TestTimeout10s(t *testing.T) {
	fc := &fakeClient{
		callDelay: 15 * time.Second,
	}
	b := newBackend(fc)
	start := time.Now()
	cred := secret.NewCredential("n", []byte("v"))
	err := b.Put(context.Background(), "sag/x/conn", cred, nil)
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("Put with delay must return error")
	}
	if elapsed >= 12*time.Second {
		t.Errorf("call took %v, want <= 12s (10s timeout)", elapsed)
	}
}

func TestErrorsHoldNoValue(t *testing.T) {
	secretValue := "supersecretvalue12345"
	fc := &fakeClient{
		createErr: fmt.Errorf("service error: %s", secretValue),
	}
	b := newBackend(fc)
	cred := secret.NewCredential("n", []byte(secretValue))
	err := b.Put(context.Background(), "sag/x/conn", cred, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), secretValue) {
		t.Errorf("error contains secret value: %v", err)
	}
}
