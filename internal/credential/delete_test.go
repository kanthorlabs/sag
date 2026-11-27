package credential

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kanthorlabs/sag/internal/podman"
	"github.com/kanthorlabs/sag/internal/prompt"
	"github.com/kanthorlabs/sag/internal/secret"
)

type deleteStorage struct {
	metas   map[string]secret.Metadata
	deleted []string
	listErr error
}

func newDeleteStorage(names []string, label string) *deleteStorage {
	ds := &deleteStorage{metas: make(map[string]secret.Metadata)}
	for _, n := range names {
		ds.metas[n] = secret.Metadata{Name: n, Labels: map[string]string{"sag.resource": label}}
	}
	return ds
}

func (s *deleteStorage) Put(_ context.Context, _ string, _ secret.Credential, _ map[string]string) error {
	return nil
}

func (s *deleteStorage) Load(_ context.Context, _ string) (secret.Credential, error) {
	return secret.Credential{}, secret.ErrNotFound
}

func (s *deleteStorage) Delete(_ context.Context, name string) error {
	s.deleted = append(s.deleted, name)
	return nil
}

func (s *deleteStorage) Metadata(_ context.Context, name string) (secret.Metadata, error) {
	if m, ok := s.metas[name]; ok {
		return m, nil
	}
	return secret.Metadata{}, secret.ErrNotFound
}

func (s *deleteStorage) List(_ context.Context, selector map[string]string) ([]secret.Metadata, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	var result []secret.Metadata
	for _, m := range s.metas {
		match := true
		for k, v := range selector {
			if m.Labels[k] != v {
				match = false
				break
			}
		}
		if match {
			result = append(result, m)
		}
	}
	return result, nil
}

type fakePodmanCmd struct {
	running bool
	execErr error
	args    [][]string
}

func (f *fakePodmanCmd) Run(_ context.Context, _ io.Reader, args ...string) ([]byte, error) {
	f.args = append(f.args, args)
	if len(args) >= 3 && args[0] == "container" && args[1] == "inspect" {
		if f.running {
			return []byte("true"), nil
		}
		return []byte("false"), nil
	}
	if len(args) >= 1 && (args[0] == "exec" || args[0] == "run") {
		return nil, f.execErr
	}
	return nil, nil
}

const yamlWithCanary = `version: 1
server:
  tls_cert: secret:sag-tls-cert
  tls_key: secret:sag-tls-key
resources:
  test-res:
    kind: fakedb
    connection: secret:sag-res-test-res.conn
    agents: [agent-a]
`

func writeTTY(t *testing.T, input string) *os.File {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	t.Cleanup(func() {
		r.Close()
		w.Close()
	})
	go func() {
		_, _ = io.WriteString(w, input)
		w.Close()
	}()
	return r
}

func TestFlagsExit2(t *testing.T) {
	for _, arg := range []string{"--yes", "--stdin", "--json"} {
		_, err := ParseDeleteArgs([]string{arg, "myres"})
		if !errors.Is(err, ErrDeleteFlagRejected) {
			t.Errorf("ParseDeleteArgs([%s myres]) error = %v; want ErrDeleteFlagRejected", arg, err)
		}
	}
}

func TestNoTTYExit3NoChange(t *testing.T) {
	dir := t.TempDir()
	writeSAGYAML(t, dir, yamlWithCanary)

	ds := newDeleteStorage([]string{"sag-res-test-res.conn"}, "test-res")
	opts := DeleteOptions{
		CfgDir:  dir,
		Backend: ds,
		Podman:  &fakePodmanCmd{},
		Out:     io.Discard,
		Err:     io.Discard,
	}

	err := Delete(context.Background(), "test-res", nil, opts)
	if !errors.Is(err, prompt.ErrNoTerminal) {
		t.Fatalf("want ErrNoTerminal, got %v", err)
	}
	if len(ds.deleted) != 0 {
		t.Errorf("secrets deleted despite no terminal: %v", ds.deleted)
	}
}

func TestWrongNameExit3(t *testing.T) {
	dir := t.TempDir()
	writeSAGYAML(t, dir, yamlWithCanary)

	ds := newDeleteStorage([]string{"sag-res-test-res.conn"}, "test-res")
	opts := DeleteOptions{
		CfgDir:  dir,
		Backend: ds,
		Podman:  &fakePodmanCmd{},
		Out:     io.Discard,
		Err:     io.Discard,
	}

	tty := writeTTY(t, "WRONGNAME\n")

	err := Delete(context.Background(), "test-res", tty, opts)
	if !errors.Is(err, ErrDeleteWrongName) {
		t.Fatalf("want ErrDeleteWrongName, got %v", err)
	}
	if len(ds.deleted) != 0 {
		t.Errorf("secrets deleted after wrong name: %v", ds.deleted)
	}
}

func TestUnreachableBackendExit1(t *testing.T) {
	dir := t.TempDir()
	writeSAGYAML(t, dir, yamlWithCanary)

	ds := &deleteStorage{metas: make(map[string]secret.Metadata), listErr: secret.ErrUnavailable}
	opts := DeleteOptions{
		CfgDir:  dir,
		Backend: ds,
		Podman:  &fakePodmanCmd{},
		Out:     io.Discard,
		Err:     io.Discard,
	}

	tty := writeTTY(t, "test-res\n")

	err := Delete(context.Background(), "test-res", tty, opts)
	if err == nil || !strings.Contains(err.Error(), "podman unreachable") {
		t.Fatalf("want unreachable error, got %v", err)
	}
}

func TestOwnsUnreferencedLabeled(t *testing.T) {
	dir := t.TempDir()
	yaml := `version: 1
server:
  tls_cert: secret:sag-tls-cert
  tls_key: secret:sag-tls-key
resources:
  test-res:
    kind: fakedb
    connection: secret:sag-res-test-res.conn
    agents: [agent-a]
`
	writeSAGYAML(t, dir, yaml)

	ds := newDeleteStorage([]string{"sag-res-test-res.conn", "sag-res-test-res.extra"}, "test-res")
	opts := DeleteOptions{
		CfgDir:  dir,
		Backend: ds,
		Podman:  &fakePodmanCmd{running: true},
		Out:     io.Discard,
		Err:     io.Discard,
	}

	tty := writeTTY(t, "test-res\n")

	err := Delete(context.Background(), "test-res", tty, opts)
	if err != nil {
		t.Fatalf("want nil, got %v", err)
	}

	deletedSet := make(map[string]bool)
	for _, d := range ds.deleted {
		deletedSet[d] = true
	}
	if !deletedSet["sag-res-test-res.conn"] {
		t.Errorf("conn secret not deleted; deleted = %v", ds.deleted)
	}
	if !deletedSet["sag-res-test-res.extra"] {
		t.Errorf("extra labeled secret not deleted; deleted = %v", ds.deleted)
	}
}

func TestSkipForeignSecret(t *testing.T) {
	dir := t.TempDir()
	yaml := `version: 1
server:
  tls_cert: secret:sag-tls-cert
  tls_key: secret:sag-tls-key
resources:
  test-res:
    kind: fakedb
    connection: secret:foreign-conn
    agents: [agent-a]
`
	writeSAGYAML(t, dir, yaml)

	ds := &deleteStorage{metas: make(map[string]secret.Metadata)}
	opts := DeleteOptions{
		CfgDir:  dir,
		Backend: ds,
		Podman:  &fakePodmanCmd{running: true},
		Out:     io.Discard,
		Err:     io.Discard,
	}

	tty := writeTTY(t, "test-res\n")

	var outBuf strings.Builder
	opts.Out = &outBuf

	err := Delete(context.Background(), "test-res", tty, opts)
	if err != nil {
		t.Fatalf("want nil, got %v", err)
	}
	if !strings.Contains(outBuf.String(), "skip") {
		t.Errorf("expected skip line for foreign secret; got %q", outBuf.String())
	}
	if len(ds.deleted) != 0 {
		t.Errorf("foreign secret deleted; deleted = %v", ds.deleted)
	}
}

func TestStoppedContainerOneShotRevoke(t *testing.T) {
	dir := t.TempDir()
	writeSAGYAML(t, dir, yamlWithCanary)

	ds := newDeleteStorage([]string{"sag-res-test-res.conn"}, "test-res")

	cmd := &fakePodmanCmd{running: false}
	opts := DeleteOptions{
		CfgDir:  dir,
		Backend: ds,
		Podman:  cmd,
		Out:     io.Discard,
		Err:     io.Discard,
		Image:   "sag:test",
	}

	tty := writeTTY(t, "test-res\n")

	err := Delete(context.Background(), "test-res", tty, opts)
	if err != nil {
		t.Fatalf("want nil, got %v", err)
	}

	sawRun := false
	for _, a := range cmd.args {
		if len(a) > 0 && a[0] == "run" {
			sawRun = true
		}
	}
	if !sawRun {
		t.Errorf("expected one-shot podman run; args = %v", cmd.args)
	}
}

func TestTolerateNoApprovalSucceeds(t *testing.T) {
	dir := t.TempDir()
	writeSAGYAML(t, dir, yamlWithCanary)

	ds := newDeleteStorage([]string{"sag-res-test-res.conn"}, "test-res")
	noApprovalErr := &podman.RunError{ExitCode: 1, Stderr: "approval: no approval record"}
	opts := DeleteOptions{
		CfgDir:  dir,
		Backend: ds,
		Podman:  &fakePodmanCmd{running: true, execErr: noApprovalErr},
		Out:     io.Discard,
		Err:     io.Discard,
	}

	tty := writeTTY(t, "test-res\n")

	err := Delete(context.Background(), "test-res", tty, opts)
	if err != nil {
		t.Fatalf("want nil when no approval record, got %v", err)
	}
	if len(ds.deleted) == 0 {
		t.Errorf("secrets not deleted despite tolerating no approval record")
	}
}

func TestExecRevokeOtherErrorFails(t *testing.T) {
	dir := t.TempDir()
	writeSAGYAML(t, dir, yamlWithCanary)

	ds := newDeleteStorage([]string{"sag-res-test-res.conn"}, "test-res")
	otherErr := &podman.RunError{ExitCode: 1, Stderr: "podman exec failed: container not found"}
	opts := DeleteOptions{
		CfgDir:  dir,
		Backend: ds,
		Podman:  &fakePodmanCmd{running: true, execErr: otherErr},
		Out:     io.Discard,
		Err:     io.Discard,
	}

	tty := writeTTY(t, "test-res\n")

	err := Delete(context.Background(), "test-res", tty, opts)
	if err == nil {
		t.Fatal("want error when exec revoke returns non-approval error, got nil")
	}
	if len(ds.deleted) != 0 {
		t.Errorf("secrets deleted despite revoke error; deleted = %v", ds.deleted)
	}
	yamlOut, _ := os.ReadFile(filepath.Join(dir, "sag.yaml"))
	if !strings.Contains(string(yamlOut), "test-res") {
		t.Errorf("sag.yaml should not have been modified when revoke fails early; got: %s", yamlOut)
	}
}

func TestSecondDeleteCompletes(t *testing.T) {
	dir := t.TempDir()
	yaml := `version: 1
server:
  tls_cert: secret:sag-tls-cert
  tls_key: secret:sag-tls-key
resources:
  other-res:
    kind: fakedb
    connection: secret:sag-res-other-res.conn
    agents: [agent-a]
`
	writeSAGYAML(t, dir, yaml)

	ds := newDeleteStorage([]string{"sag-res-test-res.conn"}, "test-res")
	opts := DeleteOptions{
		CfgDir:  dir,
		Backend: ds,
		Podman:  &fakePodmanCmd{running: true},
		Out:     io.Discard,
		Err:     io.Discard,
	}

	tty := writeTTY(t, "test-res\n")

	err := Delete(context.Background(), "test-res", tty, opts)
	if err != nil {
		t.Fatalf("second delete: want nil, got %v", err)
	}
	if len(ds.deleted) == 0 {
		t.Errorf("second delete: remaining secrets not removed; deleted = %v", ds.deleted)
	}
	yamlOut, _ := os.ReadFile(filepath.Join(dir, "sag.yaml"))
	if strings.Contains(string(yamlOut), "test-res") {
		t.Errorf("test-res still in sag.yaml after second delete")
	}
}
