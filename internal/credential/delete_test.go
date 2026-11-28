package credential

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
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
	for _, arg := range []string{"--yes", "--stdin", "--json", "--yes=false", "--stdin=false", "--json=false"} {
		_, err := ParseDeleteArgs([]string{arg, "myres"})
		if !errors.Is(err, ErrDeleteFlagRejected) {
			t.Errorf("ParseDeleteArgs([%s myres]) error = %v; want ErrDeleteFlagRejected", arg, err)
		}
	}
}

func TestNoTTYExit3NoChange(t *testing.T) {
	dir := t.TempDir()
	writeSAGYAML(t, dir, yamlWithCanary)
	before, _ := os.ReadFile(filepath.Join(dir, "sag.yaml"))

	cmd := &fakePodmanCmd{}
	ds := newDeleteStorage([]string{"sag-res-test-res.conn"}, "test-res")
	opts := DeleteOptions{
		CfgDir:  dir,
		Backend: ds,
		Podman:  cmd,
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
	for _, a := range cmd.args {
		if len(a) > 0 && (a[0] == "exec" || a[0] == "run") {
			t.Errorf("podman %v called despite no terminal", a)
		}
	}
	after, _ := os.ReadFile(filepath.Join(dir, "sag.yaml"))
	if string(before) != string(after) {
		t.Errorf("sag.yaml changed despite no terminal")
	}
}

func TestWrongNameExit3(t *testing.T) {
	dir := t.TempDir()
	writeSAGYAML(t, dir, yamlWithCanary)
	before, _ := os.ReadFile(filepath.Join(dir, "sag.yaml"))

	cmd := &fakePodmanCmd{}
	ds := newDeleteStorage([]string{"sag-res-test-res.conn"}, "test-res")
	opts := DeleteOptions{
		CfgDir:  dir,
		Backend: ds,
		Podman:  cmd,
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
	for _, a := range cmd.args {
		if len(a) > 0 && (a[0] == "exec" || a[0] == "run") {
			t.Errorf("podman %v called after wrong name", a)
		}
	}
	after, _ := os.ReadFile(filepath.Join(dir, "sag.yaml"))
	if string(before) != string(after) {
		t.Errorf("sag.yaml changed after wrong name")
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
	want := "skip foreign-conn not created by sag; not removed\n"
	if !strings.Contains(outBuf.String(), want) {
		t.Errorf("expected exact skip line %q; got %q", want, outBuf.String())
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

func TestStep3Output(t *testing.T) {
	dir := t.TempDir()
	writeSAGYAML(t, dir, yamlWithCanary)

	ds := newDeleteStorage([]string{"sag-res-test-res.conn"}, "test-res")
	opts := DeleteOptions{
		CfgDir:  dir,
		Backend: ds,
		Podman:  &fakePodmanCmd{running: true},
		Out:     io.Discard,
		Err:     io.Discard,
	}

	fds, err := syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_STREAM, 0)
	if err != nil {
		t.Fatal(err)
	}
	ttyFile := os.NewFile(uintptr(fds[0]), "tty")
	peer := os.NewFile(uintptr(fds[1]), "peer")
	t.Cleanup(func() { peer.Close() })

	if _, err := io.WriteString(peer, "test-res\n"); err != nil {
		t.Fatal(err)
	}

	if err := Delete(context.Background(), "test-res", ttyFile, opts); err != nil {
		t.Fatalf("want nil, got %v", err)
	}
	ttyFile.Close()

	out, _ := io.ReadAll(peer)
	text := string(out)
	for _, want := range []string{
		"resource: test-res",
		"kind: fakedb",
		"agents: agent-a",
		"secret: sag-res-test-res.conn",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("step 3 output missing %q; got %q", want, text)
		}
	}
}

type orderPodman struct {
	running  bool
	addEvent func(string)
}

func (p *orderPodman) Run(_ context.Context, _ io.Reader, args ...string) ([]byte, error) {
	if len(args) >= 2 && args[0] == "container" && args[1] == "inspect" {
		if p.running {
			return []byte("true"), nil
		}
		return []byte("false"), nil
	}
	if len(args) >= 1 && args[0] == "exec" {
		p.addEvent("revoke")
	}
	if len(args) >= 1 && args[0] == "run" {
		p.addEvent("revoke-oneshot")
	}
	return nil, nil
}

func TestServiceStartOrderRunning(t *testing.T) {
	dir := t.TempDir()
	writeSAGYAML(t, dir, yamlWithCanary)

	var events []string
	addEvent := func(s string) { events = append(events, s) }

	ds := newDeleteStorage([]string{"sag-res-test-res.conn"}, "test-res")
	opts := DeleteOptions{
		CfgDir:  dir,
		Backend: ds,
		Podman:  &orderPodman{running: true, addEvent: addEvent},
		Out:     io.Discard,
		Err:     io.Discard,
		ServiceStart: func(ctx context.Context, out io.Writer) error {
			data, _ := os.ReadFile(filepath.Join(dir, "sag.yaml"))
			if strings.Contains(string(data), "test-res") {
				t.Error("sag.yaml still has test-res when ServiceStart is called")
			}
			addEvent("start")
			return nil
		},
	}

	tty := writeTTY(t, "test-res\n")
	if err := Delete(context.Background(), "test-res", tty, opts); err != nil {
		t.Fatalf("want nil, got %v", err)
	}

	if len(events) < 2 || events[0] != "revoke" || events[1] != "start" {
		t.Errorf("event order = %v; want [revoke start]", events)
	}
	if len(ds.deleted) == 0 {
		t.Errorf("secrets not deleted after ServiceStart")
	}
}

func TestServiceStartNotCalledWhenStopped(t *testing.T) {
	dir := t.TempDir()
	writeSAGYAML(t, dir, yamlWithCanary)

	ds := newDeleteStorage([]string{"sag-res-test-res.conn"}, "test-res")
	startCalled := false
	opts := DeleteOptions{
		CfgDir:  dir,
		Backend: ds,
		Podman:  &fakePodmanCmd{running: false},
		Out:     io.Discard,
		Err:     io.Discard,
		Image:   "sag:test",
		ServiceStart: func(ctx context.Context, out io.Writer) error {
			startCalled = true
			return nil
		},
	}

	tty := writeTTY(t, "test-res\n")
	if err := Delete(context.Background(), "test-res", tty, opts); err != nil {
		t.Fatalf("want nil, got %v", err)
	}
	if startCalled {
		t.Errorf("ServiceStart called when container was stopped")
	}
}

func TestCrossRefSecretSkipped(t *testing.T) {
	dir := t.TempDir()
	yaml := `version: 1
server:
  tls_cert: secret:sag-tls-cert
  tls_key: secret:sag-tls-key
resources:
  test-res:
    kind: fakedb
    connection: secret:sag-res-shared.conn
    agents: [agent-a]
  other-res:
    kind: fakedb
    connection: secret:sag-res-shared.conn
    agents: [agent-b]
`
	writeSAGYAML(t, dir, yaml)

	ds := newDeleteStorage([]string{"sag-res-shared.conn"}, "test-res")
	var outBuf strings.Builder
	opts := DeleteOptions{
		CfgDir:  dir,
		Backend: ds,
		Podman:  &fakePodmanCmd{running: true},
		Out:     &outBuf,
		Err:     io.Discard,
	}

	tty := writeTTY(t, "test-res\n")
	if err := Delete(context.Background(), "test-res", tty, opts); err != nil {
		t.Fatalf("want nil, got %v", err)
	}

	if len(ds.deleted) != 0 {
		t.Errorf("cross-referenced secret was deleted; deleted = %v", ds.deleted)
	}
	want := "skip sag-res-shared.conn not created by sag; not removed\n"
	if !strings.Contains(outBuf.String(), want) {
		t.Errorf("expected skip line %q; got %q", want, outBuf.String())
	}
}

func TestResumeCase1NoOwnedSecret(t *testing.T) {
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
	startCalls := 0
	failStart := true
	opts := DeleteOptions{
		CfgDir:  dir,
		Backend: ds,
		Podman:  &fakePodmanCmd{running: true},
		Out:     io.Discard,
		Err:     io.Discard,
		ServiceStart: func(ctx context.Context, out io.Writer) error {
			startCalls++
			if failStart {
				failStart = false
				return fmt.Errorf("start failed")
			}
			return nil
		},
	}

	tty1 := writeTTY(t, "test-res\n")
	err := Delete(context.Background(), "test-res", tty1, opts)
	if err == nil || !strings.Contains(err.Error(), "service start") {
		t.Fatalf("first delete: want service start error, got %v", err)
	}

	recPath := filepath.Join(dir, "delete-pending", "test-res.json")
	fi, statErr := os.Stat(recPath)
	if statErr != nil {
		t.Fatalf("pending record not found after failure: %v", statErr)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Errorf("pending record mode = %v; want 0600", fi.Mode().Perm())
	}

	var outBuf strings.Builder
	opts.Out = &outBuf

	tty2 := writeTTY(t, "test-res\n")
	err = Delete(context.Background(), "test-res", tty2, opts)
	if err != nil {
		t.Fatalf("second delete: want nil, got %v", err)
	}

	if startCalls != 2 {
		t.Errorf("ServiceStart called %d times; want 2", startCalls)
	}
	if _, err := os.Stat(recPath); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("pending record should be removed after success")
	}
	want := "skip foreign-conn not created by sag; not removed\n"
	if !strings.Contains(outBuf.String(), want) {
		t.Errorf("expected skip line %q; got %q", want, outBuf.String())
	}
}

func TestResumeCase2StoredWasRunning(t *testing.T) {
	dir := t.TempDir()
	writeSAGYAML(t, dir, yamlWithCanary)

	ds := newDeleteStorage([]string{"sag-res-test-res.conn"}, "test-res")
	startCalls := 0
	failStart := true
	cmd := &fakePodmanCmd{running: true}
	opts := DeleteOptions{
		CfgDir:  dir,
		Backend: ds,
		Podman:  cmd,
		Out:     io.Discard,
		Err:     io.Discard,
		ServiceStart: func(ctx context.Context, out io.Writer) error {
			startCalls++
			if failStart {
				failStart = false
				return fmt.Errorf("start failed")
			}
			return nil
		},
	}

	tty1 := writeTTY(t, "test-res\n")
	err := Delete(context.Background(), "test-res", tty1, opts)
	if err == nil || !strings.Contains(err.Error(), "service start") {
		t.Fatalf("first delete: want service start error, got %v", err)
	}

	recPath := filepath.Join(dir, "delete-pending", "test-res.json")
	if _, statErr := os.Stat(recPath); statErr != nil {
		t.Fatalf("pending record not found after failure: %v", statErr)
	}

	cmd.running = false

	tty2 := writeTTY(t, "test-res\n")
	err = Delete(context.Background(), "test-res", tty2, opts)
	if err != nil {
		t.Fatalf("second delete: want nil, got %v", err)
	}

	if startCalls != 2 {
		t.Errorf("ServiceStart called %d times; want 2 (stored WasRunning=true)", startCalls)
	}
	if _, err := os.Stat(recPath); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("pending record should be removed after success")
	}
	if len(ds.deleted) == 0 {
		t.Errorf("owned secrets not deleted on retry")
	}
}

const yamlWithAWSRes = `version: 1
server:
  tls_cert: secret:sag-tls-cert
  tls_key: secret:sag-tls-key
storage:
  aws:
    region: us-east-1
    credentials: secret:sag-aws-creds
resources:
  test-res:
    kind: fakedb
    connection: aws-sm:sag/test-res/conn
    agents: [agent-a]
`

func TestDeleteUnreachableNoChange(t *testing.T) {
	dir := t.TempDir()
	writeSAGYAML(t, dir, yamlWithAWSRes)
	before, _ := os.ReadFile(filepath.Join(dir, "sag.yaml"))

	ds := newDeleteStorage(nil, "test-res")
	awsDs := &deleteStorage{metas: make(map[string]secret.Metadata), listErr: secret.ErrUnavailable}
	opts := DeleteOptions{
		CfgDir:     dir,
		Backend:    ds,
		AWSBackend: awsDs,
		Podman:     &fakePodmanCmd{},
		Out:        io.Discard,
		Err:        io.Discard,
	}

	tty := writeTTY(t, "test-res\n")

	err := Delete(context.Background(), "test-res", tty, opts)
	if err == nil || err.Error() != "backend aws-sm unreachable; no change" {
		t.Fatalf("want unreachable error, got %v", err)
	}
	if len(ds.deleted) != 0 {
		t.Errorf("podman secrets deleted despite aws backend unreachable: %v", ds.deleted)
	}
	if len(awsDs.deleted) != 0 {
		t.Errorf("aws secrets deleted despite aws backend unreachable: %v", awsDs.deleted)
	}
	after, _ := os.ReadFile(filepath.Join(dir, "sag.yaml"))
	if string(before) != string(after) {
		t.Errorf("sag.yaml changed despite aws backend unreachable")
	}
}

func TestResumeCase3StoredUnowned(t *testing.T) {
	dir := t.TempDir()
	yaml := `version: 1
server:
  tls_cert: secret:sag-tls-cert
  tls_key: secret:sag-tls-key
resources:
  test-res:
    kind: fakedb
    connection: secret:sag-res-test-res.conn
    tls_ca: secret:foreign-ca
    agents: [agent-a]
`
	writeSAGYAML(t, dir, yaml)

	ds := newDeleteStorage([]string{"sag-res-test-res.conn"}, "test-res")
	failStart := true
	opts := DeleteOptions{
		CfgDir:  dir,
		Backend: ds,
		Podman:  &fakePodmanCmd{running: true},
		Out:     io.Discard,
		Err:     io.Discard,
		ServiceStart: func(ctx context.Context, out io.Writer) error {
			if failStart {
				failStart = false
				return fmt.Errorf("start failed")
			}
			return nil
		},
	}

	tty1 := writeTTY(t, "test-res\n")
	err := Delete(context.Background(), "test-res", tty1, opts)
	if err == nil || !strings.Contains(err.Error(), "service start") {
		t.Fatalf("first delete: want service start error, got %v", err)
	}

	var outBuf strings.Builder
	opts.Out = &outBuf

	tty2 := writeTTY(t, "test-res\n")
	err = Delete(context.Background(), "test-res", tty2, opts)
	if err != nil {
		t.Fatalf("second delete: want nil, got %v", err)
	}

	want := "skip foreign-ca not created by sag; not removed\n"
	if !strings.Contains(outBuf.String(), want) {
		t.Errorf("second delete: expected skip line %q; got %q", want, outBuf.String())
	}
}

func TestDeleteAWSOwnership(t *testing.T) {
	const awsYAML = `version: 1
server:
  tls_cert: secret:sag-tls-cert
  tls_key: secret:sag-tls-key
storage:
  aws:
    region: us-east-1
    credentials: secret:sag-aws-creds
resources:
  test-res:
    kind: fakedb
    connection: aws-sm:sag/test-res/conn
    tls_ca: aws-sm:sag/test-res/ca
    agents: [agent-a]
%s`
	const otherRes = `  other-res:
    kind: fakedb
    connection: aws-sm:sag/test-res/ca
    agents: [agent-b]
`
	cases := map[string]struct {
		extra       string
		wantDeleted []string
		wantOut     string
	}{
		"owned": {
			wantDeleted: []string{"sag/test-res/ca", "sag/test-res/conn"},
		},
		"cross_referenced": {
			extra:       otherRes,
			wantDeleted: []string{"sag/test-res/conn"},
			wantOut:     "skip aws-sm:sag/test-res/ca not created by sag; not removed\n",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			writeSAGYAML(t, dir, fmt.Sprintf(awsYAML, tc.extra))
			awsDs := newDeleteStorage([]string{"sag/test-res/conn", "sag/test-res/ca"}, "test-res")
			var outBuf strings.Builder
			opts := DeleteOptions{
				CfgDir:     dir,
				Backend:    newDeleteStorage(nil, "test-res"),
				AWSBackend: awsDs,
				Podman:     &fakePodmanCmd{running: true},
				Out:        &outBuf,
				Err:        io.Discard,
			}
			if err := Delete(context.Background(), "test-res", writeTTY(t, "test-res\n"), opts); err != nil {
				t.Fatalf("Delete: %v", err)
			}
			slices.Sort(awsDs.deleted)
			if !slices.Equal(awsDs.deleted, tc.wantDeleted) {
				t.Errorf("deleted = %v; want %v", awsDs.deleted, tc.wantDeleted)
			}
			var skips []string
			for _, line := range strings.SplitAfter(outBuf.String(), "\n") {
				if strings.HasPrefix(line, "skip ") {
					skips = append(skips, line)
				}
			}
			if got := strings.Join(skips, ""); got != tc.wantOut {
				t.Errorf("skip lines = %q; want %q", got, tc.wantOut)
			}
		})
	}
}
