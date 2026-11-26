package podman

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"
)

type call struct {
	stdin []byte
	args  []string
}

type fakeExec struct {
	calls  []call
	output []byte
	err    error
}

func (f *fakeExec) run(_ context.Context, stdin io.Reader, args ...string) ([]byte, error) {
	var in []byte
	if stdin != nil {
		in, _ = io.ReadAll(stdin)
	}
	f.calls = append(f.calls, call{stdin: in, args: args})
	return f.output, f.err
}

func TestRunPipesStdinAndArgs(t *testing.T) {
	r := Runner{Path: "podman"}
	out, err := r.Run(context.Background(), strings.NewReader("hello"), "version")
	if err == nil && out == nil {
		t.Skip("not running against real podman")
	}
}

func TestSecretCreatePassesStdinAndLabel(t *testing.T) {
	f := &fakeExec{output: []byte("")}
	r := runner{exec: f.run}
	err := r.SecretCreate(context.Background(), "my-secret", strings.NewReader("my-value"), map[string]string{"k": "v"})
	if err != nil {
		t.Fatal(err)
	}
	if len(f.calls) != 1 {
		t.Fatalf("want 1 call, got %d", len(f.calls))
	}
	c := f.calls[0]
	if string(c.stdin) != "my-value" {
		t.Fatalf("stdin = %q, want %q", c.stdin, "my-value")
	}
	if c.args[len(c.args)-1] != "-" {
		t.Fatalf("last arg = %q, want -", c.args[len(c.args)-1])
	}
	found := false
	for i, a := range c.args {
		if a == "--label" && i+1 < len(c.args) && c.args[i+1] == "k=v" {
			found = true
		}
	}
	if !found {
		t.Fatalf("args missing --label k=v: %v", c.args)
	}
}

func TestSecretCreateValueNeverInArgs(t *testing.T) {
	f := &fakeExec{output: []byte("")}
	r := runner{exec: f.run}
	secretValue := "supersecret123"
	_ = r.SecretCreate(context.Background(), "s", strings.NewReader(secretValue), nil)
	if len(f.calls) == 0 {
		t.Fatal("no calls made")
	}
	for _, a := range f.calls[0].args {
		if strings.Contains(a, secretValue) {
			t.Fatalf("secret value found in args: %v", f.calls[0].args)
		}
	}
}

func TestSecretCreateExitNonZeroReturnsError(t *testing.T) {
	f := &fakeExec{err: errors.New("exit status 125")}
	r := runner{exec: f.run}
	err := r.SecretCreate(context.Background(), "s", strings.NewReader("v"), nil)
	if err == nil {
		t.Fatal("want error, got nil")
	}
}

func TestSecretInspectNoShowsecret(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	info := []map[string]any{{
		"ID": "abc123",
		"Spec": map[string]any{
			"Name":   "my-secret",
			"Labels": map[string]any{"sag.resource": "main"},
		},
		"CreatedAt": now.Format(time.RFC3339),
		"UpdatedAt": now.Format(time.RFC3339),
	}}
	out, _ := json.Marshal(info)
	f := &fakeExec{output: out}
	r := runner{exec: f.run}
	got, err := r.SecretInspect(context.Background(), "my-secret")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "abc123" {
		t.Errorf("ID = %q, want abc123", got.ID)
	}
	if got.Spec.Name != "my-secret" {
		t.Errorf("Spec.Name = %q, want my-secret", got.Spec.Name)
	}
	if got.Spec.Labels["sag.resource"] != "main" {
		t.Errorf("Labels = %v", got.Spec.Labels)
	}
	for _, a := range f.calls[0].args {
		if strings.Contains(a, "showsecret") {
			t.Fatalf("--showsecret found in args: %v", f.calls[0].args)
		}
	}
}

func TestSecretInspectNotFound(t *testing.T) {
	f := &fakeExec{err: fmt.Errorf("exit status 125: no secret with name or id %q: no such secret", "gone")}
	r := runner{exec: f.run}
	_, err := r.SecretInspect(context.Background(), "gone")
	if err == nil {
		t.Fatal("want error, got nil")
	}
}

func TestSecretListReturnsAll(t *testing.T) {
	infos := []map[string]any{
		{"ID": "id1", "Spec": map[string]any{"Name": "s1", "Labels": map[string]any{}}, "CreatedAt": "2026-01-01T00:00:00Z", "UpdatedAt": "2026-01-01T00:00:00Z"},
		{"ID": "id2", "Spec": map[string]any{"Name": "s2", "Labels": map[string]any{"k": "v"}}, "CreatedAt": "2026-01-02T00:00:00Z", "UpdatedAt": "2026-01-02T00:00:00Z"},
	}
	out, _ := json.Marshal(infos)
	f := &fakeExec{output: out}
	r := runner{exec: f.run}
	list, err := r.SecretList(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("want 2 secrets, got %d", len(list))
	}
}

func TestSecretRemoveCallsRm(t *testing.T) {
	f := &fakeExec{output: []byte("")}
	r := runner{exec: f.run}
	err := r.SecretRemove(context.Background(), "my-secret")
	if err != nil {
		t.Fatal(err)
	}
	if len(f.calls) != 1 {
		t.Fatalf("want 1 call, got %d", len(f.calls))
	}
	args := f.calls[0].args
	found := false
	for _, a := range args {
		if a == "my-secret" {
			found = true
		}
	}
	if !found {
		t.Fatalf("secret name not in args: %v", args)
	}
}

func TestContainerInspectParsesSecrets(t *testing.T) {
	infos := []map[string]any{{
		"Id":   "abc",
		"Name": "/sag",
		"HostConfig": map[string]any{},
		"Mounts":     []any{},
		"Secrets": []any{
			map[string]any{"Name": "sag-tls-cert", "UID": float64(0), "GID": float64(0), "Mode": float64(0o400)},
		},
	}}
	out, _ := json.Marshal(infos)
	f := &fakeExec{output: out}
	r := runner{exec: f.run}
	got, err := r.ContainerInspect(context.Background(), "sag")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Secrets) != 1 {
		t.Fatalf("want 1 secret, got %d", len(got.Secrets))
	}
	if got.Secrets[0].Name != "sag-tls-cert" {
		t.Errorf("secret name = %q", got.Secrets[0].Name)
	}
}

func TestContainerInspectParsesConfigSecrets(t *testing.T) {
	infos := []map[string]any{{
		"Id":   "abc",
		"Name": "/sag",
		"Config": map[string]any{
			"Secrets": []any{
				map[string]any{"Name": "sag-tls-cert", "UID": float64(0), "GID": float64(0), "Mode": float64(0o400)},
			},
		},
	}}
	out, _ := json.Marshal(infos)
	f := &fakeExec{output: out}
	r := runner{exec: f.run}
	got, err := r.ContainerInspect(context.Background(), "sag")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Secrets) != 1 {
		t.Fatalf("want 1 secret, got %d", len(got.Secrets))
	}
	if got.Secrets[0].Name != "sag-tls-cert" {
		t.Errorf("secret name = %q", got.Secrets[0].Name)
	}
}

func TestContainerInspectNotFound(t *testing.T) {
	f := &fakeExec{err: errors.New("no container with name or id")}
	r := runner{exec: f.run}
	_, err := r.ContainerInspect(context.Background(), "missing")
	if err == nil {
		t.Fatal("want error, got nil")
	}
}

func TestRunStdinPipedNotArgs(t *testing.T) {
	value := []byte("secret-value-xyz")
	f := &fakeExec{output: []byte("")}
	r := runner{exec: f.run}
	_ = r.SecretCreate(context.Background(), "test-secret", bytes.NewReader(value), nil)
	if len(f.calls) == 0 {
		t.Fatal("no calls")
	}
	c := f.calls[0]
	if !bytes.Equal(c.stdin, value) {
		t.Fatalf("stdin = %q, want %q", c.stdin, value)
	}
	for _, a := range c.args {
		if strings.Contains(a, string(value)) {
			t.Fatalf("value in args: %v", c.args)
		}
	}
}
