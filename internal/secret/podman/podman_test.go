package podman

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
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

	deleteCalls []string
	deleteErr   error

	inspectByName map[string]ipodman.SecretInfo
	inspectValue  ipodman.SecretInfo
	inspectName   string
	inspectErr    error

	listNames []string
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
		return &ipodman.RunError{ExitCode: 125, Stderr: "Error: secret name " + name + " already exists"}
	}
	return f.createErr
}

func (f *fakeRunner) SecretInspect(_ context.Context, name string) (ipodman.SecretInfo, error) {
	f.inspectName = name
	if f.inspectErr != nil {
		return ipodman.SecretInfo{}, f.inspectErr
	}
	if f.inspectByName != nil {
		if info, ok := f.inspectByName[name]; ok {
			return info, nil
		}
		return ipodman.SecretInfo{}, &ipodman.RunError{ExitCode: 125, Stderr: "no secret with name or id " + name + ": no such secret"}
	}
	return f.inspectValue, nil
}

func (f *fakeRunner) SecretList(_ context.Context) ([]string, error) {
	return f.listNames, f.listErr
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

func TestPutUnavailableReturnsErrUnavailable(t *testing.T) {
	f := &fakeRunner{createErr: &ipodman.RunError{ExitCode: -1, Stderr: "exec: not found"}}
	b := newBackend(f)
	cred := secret.NewCredential(testName, []byte("v"))
	err := b.Put(context.Background(), testName, cred, nil)
	if !errors.Is(err, secret.ErrUnavailable) {
		t.Fatalf("want ErrUnavailable, got %v", err)
	}
}

func TestDeleteAbsentIsNil(t *testing.T) {
	f := &fakeRunner{deleteErr: &ipodman.RunError{ExitCode: 125, Stderr: "no secret with name or id: no such secret"}}
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
	f := &fakeRunner{inspectErr: &ipodman.RunError{ExitCode: 125, Stderr: "no secret with name or id gone: no such secret"}}
	b := newBackend(f)
	_, err := b.Metadata(context.Background(), "gone")
	if !errors.Is(err, secret.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestMetadataEmptyInspectIsNotFound(t *testing.T) {
	f := &fakeRunner{inspectErr: ipodman.ErrEmptyInspect}
	b := newBackend(f)
	_, err := b.Metadata(context.Background(), "gone")
	if !errors.Is(err, secret.ErrNotFound) {
		t.Fatalf("want ErrNotFound for empty inspect, got %v", err)
	}
}

func TestListFiltersLabels(t *testing.T) {
	now := time.Now()
	f := &fakeRunner{
		listNames: []string{"sag-main", "unrelated", "sag-other"},
		inspectByName: map[string]ipodman.SecretInfo{
			"sag-main":  {ID: "1", Spec: ipodman.SecretSpec{Name: "sag-main", Labels: map[string]string{testLabel: "main", "other": "x"}}, CreatedAt: now},
			"unrelated": {ID: "2", Spec: ipodman.SecretSpec{Name: "unrelated", Labels: map[string]string{"foo": "bar"}}, CreatedAt: now},
			"sag-other": {ID: "3", Spec: ipodman.SecretSpec{Name: "sag-other", Labels: map[string]string{testLabel: "other"}}, CreatedAt: now},
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
	now := time.Now()
	f := &fakeRunner{
		listNames: []string{"a", "b"},
		inspectByName: map[string]ipodman.SecretInfo{
			"a": {ID: "1", Spec: ipodman.SecretSpec{Name: "a", Labels: map[string]string{"k": "v"}}, CreatedAt: now},
			"b": {ID: "2", Spec: ipodman.SecretSpec{Name: "b", Labels: map[string]string{}}, CreatedAt: now},
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

func TestListSkipsRacedDeletedSecret(t *testing.T) {
	now := time.Now()
	f := &fakeRunner{
		listNames: []string{"present", "gone"},
		inspectByName: map[string]ipodman.SecretInfo{
			"present": {ID: "1", Spec: ipodman.SecretSpec{Name: "present", Labels: map[string]string{}}, CreatedAt: now},
		},
	}
	b := newBackend(f)
	metas, err := b.List(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(metas) != 1 || metas[0].Name != "present" {
		t.Fatalf("want [present], got %v", metas)
	}
}

func TestErrorsHoldNoValue(t *testing.T) {
	secretVal := []byte("ultra-secret-value-xyz")
	f := &fakeRunner{createErr: &ipodman.RunError{ExitCode: 1, Stderr: "some podman error"}}
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

func TestLoadAppliesOwnerAndModeRule(t *testing.T) {
	dir := t.TempDir()
	name := "test-cred"
	value := []byte("my-secret-value")
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, value, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o400); err != nil {
		t.Fatal(err)
	}
	b := &Backend{runner: nil, Dir: dir, UID: os.Getuid()}
	cred, err := b.Load(context.Background(), name)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(cred.Reveal(), value) {
		t.Errorf("value = %q, want %q", cred.Reveal(), value)
	}
}

func TestLoadAbsentReturnsErrNotFound(t *testing.T) {
	dir := t.TempDir()
	b := &Backend{runner: nil, Dir: dir, UID: os.Getuid()}
	_, err := b.Load(context.Background(), "absent")
	if !errors.Is(err, secret.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}
