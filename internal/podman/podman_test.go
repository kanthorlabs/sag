package podman

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
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
	dir := t.TempDir()
	stub := filepath.Join(dir, "podman-stub")
	script := `#!/bin/sh
cat >&2
echo "stub-ok"
exit 0`
	if err := os.WriteFile(stub, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	r := Runner{Path: stub}
	stdin := strings.NewReader("hello-stdin")
	out, err := r.Run(context.Background(), stdin, "arg1", "arg2")
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if !strings.Contains(string(out), "stub-ok") {
		t.Errorf("output = %q, want stub-ok", string(out))
	}
}

func TestRunReturnsRunErrorWithStderr(t *testing.T) {
	dir := t.TempDir()
	stub := filepath.Join(dir, "podman-stub")
	script := `#!/bin/sh
echo "error details" >&2
exit 125`
	if err := os.WriteFile(stub, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	r := Runner{Path: stub}
	_, err := r.Run(context.Background(), nil, "secret", "inspect", "gone")
	if err == nil {
		t.Fatal("expected error")
	}
	var re *RunError
	if !errors.As(err, &re) {
		t.Fatalf("want *RunError, got %T: %v", err, err)
	}
	if re.ExitCode != 125 {
		t.Errorf("ExitCode = %d, want 125", re.ExitCode)
	}
	if !strings.Contains(re.Stderr, "error details") {
		t.Errorf("Stderr = %q, want 'error details'", re.Stderr)
	}
}

func TestRunErrorOnMissingProgram(t *testing.T) {
	r := Runner{Path: "/nonexistent/podman"}
	_, err := r.Run(context.Background(), nil, "version")
	if err == nil {
		t.Fatal("expected error")
	}
	var re *RunError
	if !errors.As(err, &re) {
		t.Fatalf("want *RunError, got %T: %v", err, err)
	}
	if re.ExitCode != -1 {
		t.Errorf("ExitCode = %d, want -1", re.ExitCode)
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

func TestSecretCreateSortsLabels(t *testing.T) {
	f := &fakeExec{output: []byte("")}
	r := runner{exec: f.run}
	_ = r.SecretCreate(context.Background(), "s", strings.NewReader("v"), map[string]string{"z": "1", "a": "2", "m": "3"})
	if len(f.calls) == 0 {
		t.Fatal("no calls")
	}
	var labelPairs []string
	args := f.calls[0].args
	for i, a := range args {
		if a == "--label" && i+1 < len(args) {
			labelPairs = append(labelPairs, args[i+1])
		}
	}
	if len(labelPairs) != 3 {
		t.Fatalf("want 3 labels, got %d: %v", len(labelPairs), labelPairs)
	}
	if labelPairs[0] != "a=2" || labelPairs[1] != "m=3" || labelPairs[2] != "z=1" {
		t.Errorf("labels not sorted: %v", labelPairs)
	}
}

func TestSecretCreateExitNonZeroReturnsError(t *testing.T) {
	f := &fakeExec{err: &RunError{ExitCode: 125, Stderr: "some error"}}
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
	f := &fakeExec{err: &RunError{ExitCode: 125, Stderr: "no secret with name or id gone: no such secret"}}
	r := runner{exec: f.run}
	_, err := r.SecretInspect(context.Background(), "gone")
	if err == nil {
		t.Fatal("want error, got nil")
	}
}

func TestSecretInspectEmptyReturnsNamedError(t *testing.T) {
	f := &fakeExec{output: []byte("[]")}
	r := runner{exec: f.run}
	_, err := r.SecretInspect(context.Background(), "s")
	if !errors.Is(err, ErrEmptyInspect) {
		t.Fatalf("want ErrEmptyInspect, got %v", err)
	}
}

func TestSecretListReturnsNames(t *testing.T) {
	f := &fakeExec{output: []byte("s1\ns2\n")}
	r := runner{exec: f.run}
	names, err := r.SecretList(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 2 || names[0] != "s1" || names[1] != "s2" {
		t.Fatalf("names = %v, want [s1 s2]", names)
	}
	if len(f.calls) == 0 {
		t.Fatal("no calls")
	}
	joined := strings.Join(f.calls[0].args, " ")
	if !strings.Contains(joined, "{{.Name}}") {
		t.Errorf("SecretList args missing {{.Name}}: %v", f.calls[0].args)
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
		"Id":         "abc",
		"Name":       "/sag",
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

func TestContainerInspectEmptyReturnsNamedError(t *testing.T) {
	f := &fakeExec{output: []byte("[]")}
	r := runner{exec: f.run}
	_, err := r.ContainerInspect(context.Background(), "missing")
	if !errors.Is(err, ErrEmptyInspect) {
		t.Fatalf("want ErrEmptyInspect, got %v", err)
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

func TestSecretInspectBadDateFails(t *testing.T) {
	info := []map[string]any{{
		"ID":        "abc",
		"Spec":      map[string]any{"Name": "s", "Labels": map[string]any{}},
		"CreatedAt": "not-a-date",
		"UpdatedAt": "not-a-date",
	}}
	out, _ := json.Marshal(info)
	f := &fakeExec{output: out}
	r := runner{exec: f.run}
	_, err := r.SecretInspect(context.Background(), "s")
	if err == nil {
		t.Fatal("want error for bad date format, got nil")
	}
	if errors.Is(err, ErrEmptyInspect) {
		t.Fatal("want parse error, not ErrEmptyInspect")
	}
}

func TestFromRunnerUsesRunDirectly(t *testing.T) {
	dir := t.TempDir()
	stub := filepath.Join(dir, "podman-stub")
	script := `#!/bin/sh
echo "stub-called"
exit 0`
	if err := os.WriteFile(stub, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	r := Runner{Path: stub}
	fr := fromRunner(r)
	if fr.exec == nil {
		t.Fatal("fromRunner returned runner with nil exec")
	}
	out, err := fr.exec(context.Background(), nil, "version")
	if err != nil {
		t.Fatalf("exec error: %v", err)
	}
	if !strings.Contains(string(out), "stub-called") {
		t.Errorf("output = %q, want stub-called", string(out))
	}
}

func init() {
	_ = fmt.Sprintf
}
