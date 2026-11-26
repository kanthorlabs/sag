package podman

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	ipodman "github.com/kanthorlabs/sag/internal/podman"
	"github.com/kanthorlabs/sag/internal/secret"
)

type fakeRunner struct {
	createCalls []createCall
	createErr   error
	createExist bool

	loadCalls []string
	loadValue []byte
	loadErr   error

	deleteCalls []string
	deleteErr   error

	inspectName  string
	inspectValue ipodman.SecretInfo
	inspectErr   error

	listValue []ipodman.SecretInfo
	listErr   error
}

type createCall struct {
	name   string
	stdin  []byte
	labels map[string]string
}

func (f *fakeRunner) SecretCreate(_ context.Context, name string, value io.Reader, labels map[string]string) error {
	data, _ := io.ReadAll(value)
	f.createCalls = append(f.createCalls, createCall{name: name, stdin: data, labels: labels})
	if f.createExist {
		return errors.New("secret already exists")
	}
	return f.createErr
}

func (f *fakeRunner) SecretInspect(_ context.Context, name string) (ipodman.SecretInfo, error) {
	f.inspectName = name
	return f.inspectValue, f.inspectErr
}

func (f *fakeRunner) SecretList(_ context.Context) ([]ipodman.SecretInfo, error) {
	return f.listValue, f.listErr
}

func (f *fakeRunner) SecretRemove(_ context.Context, name string) error {
	f.deleteCalls = append(f.deleteCalls, name)
	return f.deleteErr
}

const testName = "sag-main"
const testLabel = "sag.resource"

func newBackend(f *fakeRunner) *Backend {
	return New(f)
}

func TestPutValueOnStdinOnly(t *testing.T) {
	f := &fakeRunner{}
	b := newBackend(f)
	cred := secret.NewCredential(testName, []byte("the-secret"))
	err := b.Put(context.Background(), testName, cred, map[string]string{testLabel: "main"})
	if err != nil {
		t.Fatal(err)
	}
	if len(f.createCalls) != 1 {
		t.Fatalf("want 1 create call, got %d", len(f.createCalls))
	}
	c := f.createCalls[0]
	if !bytes.Equal(c.stdin, cred.Reveal()) {
		t.Fatalf("stdin = %q, want %q", c.stdin, cred.Reveal())
	}
	if c.labels[testLabel] != "main" {
		t.Errorf("label missing: %v", c.labels)
	}
}

func TestPutExistsReturnsErrExists(t *testing.T) {
	f := &fakeRunner{createExist: true}
	b := newBackend(f)
	cred := secret.NewCredential(testName, []byte("v"))
	err := b.Put(context.Background(), testName, cred, nil)
	if !errors.Is(err, secret.ErrExists) {
		t.Fatalf("want ErrExists, got %v", err)
	}
}

func TestDeleteAbsentIsNil(t *testing.T) {
	f := &fakeRunner{deleteErr: errors.New("no secret with name or id: not found")}
	b := newBackend(f)
	err := b.Delete(context.Background(), "absent")
	if err != nil {
		t.Fatalf("want nil for absent, got %v", err)
	}
}

func TestDeletePresentIsNil(t *testing.T) {
	f := &fakeRunner{}
	b := newBackend(f)
	err := b.Delete(context.Background(), testName)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.deleteCalls) != 1 || f.deleteCalls[0] != testName {
		t.Fatalf("delete calls = %v", f.deleteCalls)
	}
}

func TestMetadataNoShowsecret(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	f := &fakeRunner{
		inspectValue: ipodman.SecretInfo{
			ID:        "abc",
			Spec:      ipodman.SecretSpec{Name: testName, Labels: map[string]string{testLabel: "main"}},
			CreatedAt: now,
		},
	}
	b := newBackend(f)
	meta, err := b.Metadata(context.Background(), testName)
	if err != nil {
		t.Fatal(err)
	}
	if meta.Name != testName {
		t.Errorf("Name = %q", meta.Name)
	}
	if meta.Labels[testLabel] != "main" {
		t.Errorf("Labels = %v", meta.Labels)
	}
	if !meta.CreatedAt.Equal(now) {
		t.Errorf("CreatedAt = %v, want %v", meta.CreatedAt, now)
	}
	if f.inspectName != testName {
		t.Errorf("inspected name = %q", f.inspectName)
	}
}

func TestMetadataNotFound(t *testing.T) {
	f := &fakeRunner{inspectErr: errors.New("no secret with name")}
	b := newBackend(f)
	_, err := b.Metadata(context.Background(), "gone")
	if !errors.Is(err, secret.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestListFiltersLabels(t *testing.T) {
	f := &fakeRunner{
		listValue: []ipodman.SecretInfo{
			{ID: "1", Spec: ipodman.SecretSpec{Name: "sag-main", Labels: map[string]string{testLabel: "main", "other": "x"}}, CreatedAt: time.Now()},
			{ID: "2", Spec: ipodman.SecretSpec{Name: "unrelated", Labels: map[string]string{"foo": "bar"}}, CreatedAt: time.Now()},
			{ID: "3", Spec: ipodman.SecretSpec{Name: "sag-other", Labels: map[string]string{testLabel: "other"}}, CreatedAt: time.Now()},
		},
	}
	b := newBackend(f)
	metas, err := b.List(context.Background(), map[string]string{testLabel: "main"})
	if err != nil {
		t.Fatal(err)
	}
	if len(metas) != 1 {
		t.Fatalf("want 1 result, got %d: %v", len(metas), metas)
	}
	if metas[0].Name != "sag-main" {
		t.Errorf("Name = %q", metas[0].Name)
	}
}

func TestListNoSelectorReturnsAll(t *testing.T) {
	f := &fakeRunner{
		listValue: []ipodman.SecretInfo{
			{ID: "1", Spec: ipodman.SecretSpec{Name: "a", Labels: map[string]string{"k": "v"}}, CreatedAt: time.Now()},
			{ID: "2", Spec: ipodman.SecretSpec{Name: "b", Labels: map[string]string{}}, CreatedAt: time.Now()},
		},
	}
	b := newBackend(f)
	metas, err := b.List(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(metas) != 2 {
		t.Fatalf("want 2, got %d", len(metas))
	}
}

func TestErrorsHoldNoValue(t *testing.T) {
	secretVal := []byte("ultra-secret-value-xyz")
	f := &fakeRunner{createErr: errors.New("some podman error")}
	b := newBackend(f)
	cred := secret.NewCredential(testName, secretVal)
	err := b.Put(context.Background(), testName, cred, nil)
	if err == nil {
		t.Fatal("want error")
	}
	if strings.Contains(err.Error(), string(secretVal)) {
		t.Fatalf("error contains secret value: %v", err)
	}
}
