package credential

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/kanthorlabs/sag/internal/config"
	"github.com/kanthorlabs/sag/internal/core"
	"github.com/kanthorlabs/sag/internal/prompt"
	"github.com/kanthorlabs/sag/internal/secret"
)

const baseYAML = `version: 1
server:
  tls_cert: secret:sag-tls-cert
  tls_key: secret:sag-tls-key
resources:
  existing-res:
    kind: fakedb
    connection: secret:sag-existing-res.conn
    agents: [agent-a]
`

type fakeKind struct {
	notes core.CredentialNotes
	err   error
}

func (f fakeKind) CredentialPrompt() string { return "DSN" }
func (f fakeKind) ValidateCredential(_ secret.Credential) (core.CredentialNotes, error) {
	return f.notes, f.err
}
func (f fakeKind) CheckCredential(_ context.Context, _ core.ResourceConfig, _ secret.Credential) error {
	return nil
}

type fakeStorage struct {
	mu      sync.Mutex
	entries map[string]secret.Credential
	labels  map[string]map[string]string
	deleted []string
}

func newFakeStorage() *fakeStorage {
	return &fakeStorage{
		entries: make(map[string]secret.Credential),
		labels:  make(map[string]map[string]string),
	}
}

func (s *fakeStorage) Put(_ context.Context, name string, value secret.Credential, lbls map[string]string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries[name] = value
	s.labels[name] = lbls
	return nil
}

func (s *fakeStorage) Delete(_ context.Context, name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deleted = append(s.deleted, name)
	delete(s.entries, name)
	return nil
}

func (s *fakeStorage) Metadata(_ context.Context, name string) (secret.Metadata, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.entries[name]; ok {
		return secret.Metadata{Name: name}, nil
	}
	return secret.Metadata{}, secret.ErrNotFound
}

func (s *fakeStorage) Load(_ context.Context, name string) (secret.Credential, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if c, ok := s.entries[name]; ok {
		return c, nil
	}
	return secret.Credential{}, secret.ErrNotFound
}

func (s *fakeStorage) List(_ context.Context, _ map[string]string) ([]secret.Metadata, error) {
	return nil, nil
}

func writeSAGYAML(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "sag.yaml"), []byte(content), 0o644); err != nil {
		t.Fatalf("write sag.yaml: %v", err)
	}
}

func noopServiceStart(_ context.Context, _ io.Writer) error { return nil }
func fakeContainerRunning(_ context.Context) (bool, error)  { return true, nil }

func baseOpts(t *testing.T) CreateOptions {
	t.Helper()
	store := newFakeStorage()
	return CreateOptions{
		Name:         "mydb",
		Kind:         "fakedb",
		Agents:       []string{"agent-a"},
		TLS:          core.TLSDisable,
		CfgDir:       t.TempDir(),
		Storage:      store,
		KindLookup:   func(k string) (core.CredentialKind, bool) { return fakeKind{}, k == "fakedb" },
		CheckCred:    func(_ context.Context, _ string, _ core.TLSMode, _ secret.Credential, _ []byte) error { return nil },
		ServiceStart: noopServiceStart,
		ContainerRun: fakeContainerRunning,
		OpenTTY:      func() (*os.File, error) { return nil, errors.New("no tty in tests") },
		StdinReader:  strings.NewReader("postgres://user:pass@db.example.com/mydb"),
		IsStdinTerm:  false,
		Stdin:        true,
		Out:          io.Discard,
		Err:          io.Discard,
	}
}

func TestNameRegexp(t *testing.T) {
	valid := []string{"a", "abc", "a-b-c", "a1", "z09", strings.Repeat("a", 63)}
	for _, n := range valid {
		if err := validateName(n); err != nil {
			t.Errorf("validateName(%q): want nil, got %v", n, err)
		}
	}
	invalid := []string{"", "A", "1abc", "-abc", strings.Repeat("a", 64)}
	for _, n := range invalid {
		if err := validateName(n); err == nil {
			t.Errorf("validateName(%q): want error, got nil", n)
		}
	}
}

func TestValidationOrder(t *testing.T) {
	dir := t.TempDir()
	writeSAGYAML(t, dir, baseYAML)

	store := newFakeStorage()
	store.entries["sag-res-mydb.conn"] = secret.NewCredential("x", []byte("y"))

	call := func(name, kind string, agents []string) error {
		opts := CreateOptions{
			Name:         name,
			Kind:         kind,
			Agents:       agents,
			TLS:          core.TLSDisable,
			CfgDir:       dir,
			Storage:      store,
			KindLookup:   func(k string) (core.CredentialKind, bool) { return fakeKind{}, k == "fakedb" },
			CheckCred:    func(_ context.Context, _ string, _ core.TLSMode, _ secret.Credential, _ []byte) error { return nil },
			ServiceStart: noopServiceStart,
			ContainerRun: fakeContainerRunning,
			OpenTTY:      func() (*os.File, error) { return nil, errors.New("no tty") },
			StdinReader:  strings.NewReader("postgres://user:pass@db.example.com/mydb"),
			IsStdinTerm:  false,
			Stdin:        true,
			Out:          io.Discard,
			Err:          io.Discard,
		}
		return Create(context.Background(), opts)
	}

	if err := call("BADNAME", "fakedb", []string{"agent-a"}); err == nil || !strings.Contains(err.Error(), "invalid name") {
		t.Fatalf("want invalid name error, got %v", err)
	}

	if err := call("existing-res", "fakedb", []string{"agent-a"}); err == nil || !strings.Contains(err.Error(), "resource existing-res exists") {
		t.Fatalf("want resource-exists error, got %v", err)
	}

	if err := call("mydb", "fakedb", []string{"agent-a"}); err == nil || !strings.Contains(err.Error(), "secret sag-res-mydb.conn exists") {
		t.Fatalf("want secret-exists error, got %v", err)
	}

	// S6: break rules 6+7 simultaneously (no agents, long description) — rule 6 fires first
	longDesc := strings.Repeat("x", 501)
	if err := call("newres", "fakedb", nil); err == nil || !strings.Contains(err.Error(), "at least one --agent is required") {
		t.Fatalf("rules 6+7: want agent error, got %v", err)
	}

	// S6: break rules 7+8 simultaneously (long description, bad TLS CA) — rule 7 fires first
	type overrideOpts struct {
		name        string
		kind        string
		agents      []string
		description string
		tlsCAPem    []byte
	}
	callFull := func(o overrideOpts) error {
		opts := CreateOptions{
			Name:         o.name,
			Kind:         o.kind,
			Agents:       o.agents,
			Description:  o.description,
			TLS:          core.TLSDisable,
			TLSCAPem:     o.tlsCAPem,
			CfgDir:       dir,
			Storage:      store,
			KindLookup:   func(k string) (core.CredentialKind, bool) { return fakeKind{}, k == "fakedb" },
			CheckCred:    func(_ context.Context, _ string, _ core.TLSMode, _ secret.Credential, _ []byte) error { return nil },
			ServiceStart: noopServiceStart,
			ContainerRun: fakeContainerRunning,
			OpenTTY:      func() (*os.File, error) { return nil, errors.New("no tty") },
			StdinReader:  strings.NewReader("val\n"),
			IsStdinTerm:  false,
			Stdin:        true,
			Out:          io.Discard,
			Err:          io.Discard,
		}
		return Create(context.Background(), opts)
	}
	_ = longDesc
	if err := callFull(overrideOpts{name: "newres2", kind: "fakedb", agents: []string{"a"}, description: strings.Repeat("y", 501), tlsCAPem: []byte("not-a-cert")}); err == nil || !strings.Contains(err.Error(), "--description: at most 500 characters") {
		t.Fatalf("rules 7+8: want description error, got %v", err)
	}
}

func TestResourceExists(t *testing.T) {
	dir := t.TempDir()
	writeSAGYAML(t, dir, baseYAML)
	opts := baseOpts(t)
	opts.Name = "existing-res"
	opts.CfgDir = dir

	err := Create(context.Background(), opts)
	if err == nil || !strings.Contains(err.Error(), "resource existing-res exists") {
		t.Fatalf("want resource-exists error, got %v", err)
	}
}

func TestSecretExists(t *testing.T) {
	dir := t.TempDir()
	writeSAGYAML(t, dir, baseYAML)
	opts := baseOpts(t)
	opts.CfgDir = dir

	store := opts.Storage.(*fakeStorage)
	store.entries["sag-res-mydb.conn"] = secret.NewCredential("x", []byte("y"))

	err := Create(context.Background(), opts)
	if err == nil || !strings.Contains(err.Error(), "secret sag-res-mydb.conn exists") {
		t.Fatalf("want secret-exists error, got %v", err)
	}
}

func TestStdinLimits(t *testing.T) {
	t.Run("too long", func(t *testing.T) {
		data := strings.Repeat("x", maxStdinBytes+1)
		_, err := readStdin(strings.NewReader(data), false)
		if err == nil || !strings.Contains(err.Error(), "64 KiB") {
			t.Fatalf("want 64 KiB error, got %v", err)
		}
	})
	t.Run("nul byte", func(t *testing.T) {
		_, err := readStdin(strings.NewReader("abc\x00def"), false)
		if err == nil || !strings.Contains(err.Error(), "NUL") {
			t.Fatalf("want NUL error, got %v", err)
		}
	})
	t.Run("inner linebreak", func(t *testing.T) {
		_, err := readStdin(strings.NewReader("abc\ndef"), false)
		if err == nil || !strings.Contains(err.Error(), "inner line break") {
			t.Fatalf("want inner linebreak error, got %v", err)
		}
	})
	t.Run("strips trailing newline", func(t *testing.T) {
		cred, err := readStdin(strings.NewReader("myvalue\n"), false)
		if err != nil {
			t.Fatalf("readStdin: %v", err)
		}
		if got := string(cred.Reveal()); got != "myvalue" {
			t.Fatalf("want %q, got %q", "myvalue", got)
		}
	})
	t.Run("strips trailing crlf", func(t *testing.T) {
		cred, err := readStdin(strings.NewReader("myvalue\r\n"), false)
		if err != nil {
			t.Fatalf("readStdin: %v", err)
		}
		if got := string(cred.Reveal()); got != "myvalue" {
			t.Fatalf("want %q, got %q", "myvalue", got)
		}
	})
}

func TestStdinTerminalRefused(t *testing.T) {
	dir := t.TempDir()
	writeSAGYAML(t, dir, baseYAML)
	opts := baseOpts(t)
	opts.CfgDir = dir
	opts.Stdin = true
	opts.IsStdinTerm = true

	err := Create(context.Background(), opts)
	if !errors.Is(err, prompt.ErrNoTerminal) {
		t.Fatalf("want ErrNoTerminal, got %v", err)
	}
}

func TestPipedWithoutStdinExit3(t *testing.T) {
	dir := t.TempDir()
	writeSAGYAML(t, dir, baseYAML)
	opts := baseOpts(t)
	opts.CfgDir = dir
	opts.Stdin = false
	opts.IsStdinTerm = false
	opts.OpenTTY = func() (*os.File, error) { return nil, errors.New("no tty: injected") }

	err := Create(context.Background(), opts)
	if !errors.Is(err, prompt.ErrNoTerminal) {
		t.Fatalf("want ErrNoTerminal when OpenTTY fails, got %v", err)
	}
}

func TestKeepsOtherYAML(t *testing.T) {
	dir := t.TempDir()
	writeSAGYAML(t, dir, baseYAML)

	opts := baseOpts(t)
	opts.CfgDir = dir

	if err := Create(context.Background(), opts); err != nil {
		t.Fatalf("Create: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "sag.yaml"))
	if err != nil {
		t.Fatalf("read sag.yaml: %v", err)
	}
	got := string(data)
	if !strings.Contains(got, "existing-res") {
		t.Errorf("sag.yaml lost existing-res after create:\n%s", got)
	}
	if !strings.Contains(got, "mydb") {
		t.Errorf("sag.yaml missing mydb after create:\n%s", got)
	}

	cfg, parseErr := config.Parse(data)
	if parseErr != nil {
		t.Fatalf("parse sag.yaml: %v", parseErr)
	}
	if cfg.Version != 1 {
		t.Errorf("version = %d; want 1", cfg.Version)
	}
	if cfg.Server.TLSCert == "" {
		t.Error("server.tls_cert missing after create")
	}
	oldRes, ok := cfg.Resources["existing-res"]
	if !ok {
		t.Error("existing-res missing from resources")
	} else if oldRes.Kind != "fakedb" {
		t.Errorf("existing-res.kind = %q; want fakedb", oldRes.Kind)
	}
	newRes, ok := cfg.Resources["mydb"]
	if !ok {
		t.Fatal("mydb missing from resources")
	}
	if newRes.Kind != "fakedb" {
		t.Errorf("mydb.kind = %q; want fakedb", newRes.Kind)
	}
	if len(newRes.Agents) == 0 || newRes.Agents[0] != "agent-a" {
		t.Errorf("mydb.agents = %v; want [agent-a]", newRes.Agents)
	}
	if string(newRes.Connection) == "" {
		t.Error("mydb.connection missing")
	}
}

func TestArgsHoldNoValue(t *testing.T) {
	dir := t.TempDir()
	writeSAGYAML(t, dir, baseYAML)

	const secretValue = "super-secret-dsn-value-99"

	var outBuf strings.Builder
	var errBuf strings.Builder

	store := newFakeStorage()
	var putArgs []string

	opts := CreateOptions{
		Name:       "mydb",
		Kind:       "fakedb",
		Agents:     []string{"agent-a"},
		TLS:        core.TLSDisable,
		CfgDir:     dir,
		Storage:    store,
		KindLookup: func(k string) (core.CredentialKind, bool) { return fakeKind{}, k == "fakedb" },
		CheckCred: func(_ context.Context, kind string, _ core.TLSMode, cred secret.Credential, _ []byte) error {
			putArgs = append(putArgs, kind)
			return nil
		},
		ServiceStart: noopServiceStart,
		ContainerRun: fakeContainerRunning,
		OpenTTY:      func() (*os.File, error) { return nil, errors.New("no tty") },
		StdinReader:  strings.NewReader(secretValue + "\n"),
		IsStdinTerm:  false,
		Stdin:        true,
		Out:          &outBuf,
		Err:          &errBuf,
	}

	if err := Create(context.Background(), opts); err != nil {
		t.Fatalf("Create: %v", err)
	}

	for _, a := range putArgs {
		if strings.Contains(a, secretValue) {
			t.Errorf("secret value found in CheckCred kind arg: %q", a)
		}
	}
	for lbl, vals := range store.labels {
		for k, v := range vals {
			if strings.Contains(v, secretValue) {
				t.Errorf("secret value found in label %s/%s: %q", lbl, k, v)
			}
		}
	}
	if strings.Contains(outBuf.String(), secretValue) {
		t.Errorf("secret value found in stdout: %q", outBuf.String())
	}
	if strings.Contains(errBuf.String(), secretValue) {
		t.Errorf("secret value found in stderr: %q", errBuf.String())
	}
	conn := store.entries["sag-res-mydb.conn"]
	if string(conn.Reveal()) != secretValue {
		t.Errorf("connection secret value = %q; want %q (value must reach storage)", string(conn.Reveal()), secretValue)
	}
}

func TestAWSStorageRuleOrder(t *testing.T) {
	storageErr := errors.New("credential: backend aws-sm needs storage.aws in sag.yaml")
	cases := map[string]struct {
		mutate func(*CreateOptions)
		want   string
	}{
		"rule1_first":     {func(o *CreateOptions) { o.Name = "BAD" }, "invalid name"},
		"rule4_first":     {func(o *CreateOptions) { o.Agents = nil }, "at least one --agent is required"},
		"rule5_first":     {func(o *CreateOptions) { o.Description = strings.Repeat("y", 501) }, "--description: at most 500 characters"},
		"rule6_before_7":  {func(o *CreateOptions) { o.ContainerRun = func(context.Context) (bool, error) { return false, nil } }, storageErr.Error()},
		"rule6_no_secret": {func(o *CreateOptions) {}, storageErr.Error()},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			opts := baseOpts(t)
			opts.Backend = secret.BackendAWSSM
			opts.Storage = nil
			opts.StorageErr = storageErr
			tc.mutate(&opts)
			err := Create(context.Background(), opts)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Create = %v; want %q", err, tc.want)
			}
		})
	}
}
