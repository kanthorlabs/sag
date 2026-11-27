package credential

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"maps"
	"math/big"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/kanthorlabs/sag/internal/podman"
	"github.com/kanthorlabs/sag/internal/secret"
)

type fakeBackend struct {
	metas      map[string]secret.Metadata
	loadCalled bool
}

func (f *fakeBackend) Put(_ context.Context, name string, _ secret.Credential, labels map[string]string) error {
	return nil
}

func (f *fakeBackend) Load(_ context.Context, _ string) (secret.Credential, error) {
	f.loadCalled = true
	return secret.Credential{}, errors.New("Load must not be called")
}

func (f *fakeBackend) Delete(_ context.Context, _ string) error {
	return nil
}

func (f *fakeBackend) Metadata(_ context.Context, name string) (secret.Metadata, error) {
	if m, ok := f.metas[name]; ok {
		return m, nil
	}
	return secret.Metadata{}, secret.ErrNotFound
}

func (f *fakeBackend) List(_ context.Context, _ map[string]string) ([]secret.Metadata, error) {
	return nil, nil
}

type fakeUnavailableBackend struct{}

func (f *fakeUnavailableBackend) Put(_ context.Context, _ string, _ secret.Credential, _ map[string]string) error {
	return secret.ErrUnavailable
}
func (f *fakeUnavailableBackend) Load(_ context.Context, _ string) (secret.Credential, error) {
	return secret.Credential{}, secret.ErrUnavailable
}
func (f *fakeUnavailableBackend) Delete(_ context.Context, _ string) error {
	return secret.ErrUnavailable
}
func (f *fakeUnavailableBackend) Metadata(_ context.Context, _ string) (secret.Metadata, error) {
	return secret.Metadata{}, secret.ErrUnavailable
}
func (f *fakeBackend) podmanInspector() podmanInspector { return nil }

func (f *fakeUnavailableBackend) List(_ context.Context, _ map[string]string) ([]secret.Metadata, error) {
	return nil, secret.ErrUnavailable
}

type fakePodman struct {
	secrets []podman.ContainerSecret
	running bool
}

func (f *fakePodman) ContainerInspect(_ context.Context, name string) (podman.ContainerInfo, error) {
	return podman.ContainerInfo{Name: name, Secrets: f.secrets}, nil
}

func (f *fakePodman) Run(_ context.Context, _ io.Reader, args ...string) ([]byte, error) {
	for i, a := range args {
		if a == "--format" && i+1 < len(args) && args[i+1] == "{{.State.Running}}" {
			if f.running {
				return []byte("true\n"), nil
			}
			return []byte("false\n"), nil
		}
	}
	return nil, nil
}

func writeSAGYaml(t *testing.T, dir, content string) string {
	t.Helper()
	path := filepath.Join(dir, "sag.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write sag.yaml: %v", err)
	}
	return path
}

const minimalYAML = `version: 1
server:
  tls_cert: "secret:sag-tls-cert"
  tls_key: "secret:sag-tls-key"
resources:
  canary:
    kind: postgres
    description: "test canary"
    connection: "secret:sag-res-canary.conn"
    tls: "verify-full"
    agents:
      - claude-code
`

func TestJSONFieldSetExact(t *testing.T) {
	dir := t.TempDir()
	yamlPath := writeSAGYaml(t, dir, minimalYAML)

	ts := time.Now()
	be := &fakeBackend{
		metas: map[string]secret.Metadata{
			"sag-res-canary.conn": {
				Name:      "sag-res-canary.conn",
				Labels:    map[string]string{"sag.resource": "canary"},
				CreatedAt: ts,
			},
		},
	}
	pm := &fakePodman{secrets: []podman.ContainerSecret{{Name: "sag-res-canary.conn"}}, running: true}

	entries, err := List(context.Background(), ListOptions{
		SAGYaml: yamlPath,
		Backend: be,
		Podman:  pm,
		Stderr:  io.Discard,
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("want 1 entry, got %d", len(entries))
	}

	raw, err := json.Marshal(entries[0])
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	wantListKeys := map[string]bool{
		"name": true, "kind": true, "description": true,
		"tls": true, "agents": true, "secrets": true,
	}
	gotKeys := map[string]bool{}
	for k := range obj {
		gotKeys[k] = true
	}
	if !maps.Equal(wantListKeys, gotKeys) {
		t.Errorf("list entry keys: got %v, want %v", sortedKeys(gotKeys), sortedKeys(wantListKeys))
	}

	secsRaw, _ := json.Marshal(entries[0].Secrets[0])
	var seObj map[string]any
	if err := json.Unmarshal(secsRaw, &seObj); err != nil {
		t.Fatalf("secret entry marshal: %v", err)
	}
	wantSecretKeys := map[string]bool{
		"name": true, "backend": true, "role": true,
		"exists": true, "owned": true, "created_at": true, "mounted": true,
	}
	gotSecretKeys := map[string]bool{}
	for k := range seObj {
		gotSecretKeys[k] = true
	}
	if !maps.Equal(wantSecretKeys, gotSecretKeys) {
		t.Errorf("secret entry keys: got %v, want %v", sortedKeys(gotSecretKeys), sortedKeys(wantSecretKeys))
	}

	inspectEntry, err := Inspect(context.Background(), "canary", InspectOptions{
		SAGYaml: yamlPath,
		Backend: be,
		Podman:  pm,
		Stderr:  io.Discard,
	})
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}

	rawInspect, err := json.Marshal(inspectEntry)
	if err != nil {
		t.Fatalf("marshal inspect: %v", err)
	}
	var inspObj map[string]any
	if err := json.Unmarshal(rawInspect, &inspObj); err != nil {
		t.Fatalf("unmarshal inspect: %v", err)
	}
	wantInspectKeys := map[string]bool{
		"name": true, "kind": true, "description": true,
		"tls": true, "agents": true, "secrets": true, "container_running": true,
	}
	gotInspectKeys := map[string]bool{}
	for k := range inspObj {
		gotInspectKeys[k] = true
	}
	if !maps.Equal(wantInspectKeys, gotInspectKeys) {
		t.Errorf("inspect entry keys: got %v, want %v", sortedKeys(gotInspectKeys), sortedKeys(wantInspectKeys))
	}

	secsData := inspObj["secrets"].([]any)
	if len(secsData) == 0 {
		t.Fatal("inspect: no secrets in entry")
	}
	secMap := secsData[0].(map[string]any)
	if _, ok := secMap["mounted"]; !ok {
		t.Error("inspect secret entry: missing mounted field for podman backend")
	}
}

func TestNoLoadCall(t *testing.T) {
	dir := t.TempDir()
	yamlPath := writeSAGYaml(t, dir, minimalYAML)
	ts := time.Now()

	for _, name := range []string{"not-found backend", "found backend"} {
		metas := map[string]secret.Metadata{}
		if name == "found backend" {
			metas["sag-res-canary.conn"] = secret.Metadata{
				Name:      "sag-res-canary.conn",
				Labels:    map[string]string{"sag.resource": "canary"},
				CreatedAt: ts,
			}
		}
		be := &fakeBackend{metas: metas}
		pm := &fakePodman{}

		_, err := List(context.Background(), ListOptions{
			SAGYaml: yamlPath,
			Backend: be,
			Podman:  pm,
			Stderr:  io.Discard,
		})
		if err != nil {
			t.Fatalf("%s: List: %v", name, err)
		}
		if be.loadCalled {
			t.Errorf("%s: List called backend.Load; it must not", name)
		}

		be.loadCalled = false
		_, err = Inspect(context.Background(), "canary", InspectOptions{
			SAGYaml: yamlPath,
			Backend: be,
			Podman:  pm,
			Stderr:  io.Discard,
		})
		if err != nil {
			t.Fatalf("%s: Inspect: %v", name, err)
		}
		if be.loadCalled {
			t.Errorf("%s: Inspect called backend.Load; it must not", name)
		}
	}
}

func TestUnreachableBackendWarns(t *testing.T) {
	dir := t.TempDir()
	yamlPath := writeSAGYaml(t, dir, minimalYAML)
	be := &fakeUnavailableBackend{}
	var buf bytes.Buffer

	entries, err := List(context.Background(), ListOptions{
		SAGYaml: yamlPath,
		Backend: be,
		Podman:  &fakePodman{},
		Stderr:  &buf,
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("want entries even on unreachable backend")
	}
	if entries[0].Secrets[0].Exists != nil {
		t.Errorf("exists must be nil when backend unreachable, got %v", entries[0].Secrets[0].Exists)
	}
	warnLine := buf.String()
	if !strings.Contains(warnLine, "warn") || !strings.Contains(warnLine, "unreachable") {
		t.Errorf("want warn unreachable in stderr, got: %q", warnLine)
	}
}

func TestExpiryWarn(t *testing.T) {
	dir := t.TempDir()
	caPath := filepath.Join(dir, "ca.pem")

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("gen key: %v", err)
	}
	serial, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	now := time.Now()
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: "SAG CA"},
		NotBefore:    now.Add(-24 * time.Hour),
		NotAfter:     now.Add(10 * 24 * time.Hour),
		IsCA:         true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create cert: %v", err)
	}
	var buf bytes.Buffer
	if err := pem.Encode(&buf, &pem.Block{Type: "CERTIFICATE", Bytes: der}); err != nil {
		t.Fatalf("pem encode: %v", err)
	}
	if err := os.WriteFile(caPath, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("write ca.pem: %v", err)
	}

	dir2 := t.TempDir()
	yamlPath := writeSAGYaml(t, dir2, minimalYAML)
	be := &fakeBackend{metas: map[string]secret.Metadata{}}

	var stderr bytes.Buffer
	_, err = List(context.Background(), ListOptions{
		SAGYaml: yamlPath,
		CAPem:   caPath,
		Backend: be,
		Podman:  &fakePodman{},
		Stderr:  &stderr,
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	got := stderr.String()
	if !strings.Contains(got, "warn") || !strings.Contains(got, "endpoint") || !strings.Contains(got, "expires") {
		t.Errorf("want expiry warn in stderr, got: %q", got)
	}
	if !strings.Contains(got, "sag bootstrap") {
		t.Errorf("want 'sag bootstrap' in warn line, got: %q", got)
	}

	var noWarnBuf bytes.Buffer
	key2, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	serial2, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	tmpl2 := &x509.Certificate{
		SerialNumber: serial2,
		Subject:      pkix.Name{CommonName: "SAG CA"},
		NotBefore:    now.Add(-24 * time.Hour),
		NotAfter:     now.Add(60 * 24 * time.Hour),
		IsCA:         true,
	}
	der2, _ := x509.CreateCertificate(rand.Reader, tmpl2, tmpl2, &key2.PublicKey, key2)
	var buf2 bytes.Buffer
	pem.Encode(&buf2, &pem.Block{Type: "CERTIFICATE", Bytes: der2})
	caPath2 := filepath.Join(dir, "ca2.pem")
	os.WriteFile(caPath2, buf2.Bytes(), 0o644)

	_, err = List(context.Background(), ListOptions{
		SAGYaml: yamlPath,
		CAPem:   caPath2,
		Backend: be,
		Podman:  &fakePodman{},
		Stderr:  &noWarnBuf,
	})
	if err != nil {
		t.Fatalf("List no-warn: %v", err)
	}
	if strings.Contains(noWarnBuf.String(), "expires") {
		t.Errorf("want no expiry warn for cert with 60 days left, got: %q", noWarnBuf.String())
	}
}

func TestInspectUnknownExit1(t *testing.T) {
	dir := t.TempDir()
	yamlPath := writeSAGYaml(t, dir, minimalYAML)
	be := &fakeBackend{metas: map[string]secret.Metadata{}}

	_, err := Inspect(context.Background(), "unknown-resource", InspectOptions{
		SAGYaml: yamlPath,
		Backend: be,
		Podman:  &fakePodman{},
		Stderr:  io.Discard,
	})
	if err == nil {
		t.Fatal("Inspect of unknown resource: want error, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("want 'not found' in error, got: %q", err.Error())
	}
}

func TestAgentsAndSecretsNeverNull(t *testing.T) {
	const yamlNoAgents = `version: 1
server:
  tls_cert: "secret:sag-tls-cert"
  tls_key: "secret:sag-tls-key"
resources:
  res1:
    kind: postgres
    connection: "secret:sag-res-res1.conn"
    tls: "verify-full"
    agents:
      - claude-code
`
	dir := t.TempDir()
	yamlPath := writeSAGYaml(t, dir, yamlNoAgents)
	be := &fakeBackend{metas: map[string]secret.Metadata{}}

	entries, err := List(context.Background(), ListOptions{
		SAGYaml: yamlPath,
		Backend: be,
		Podman:  &fakePodman{},
		Stderr:  io.Discard,
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("no entries")
	}

	raw, _ := json.Marshal(entries[0])
	if strings.Contains(string(raw), `"agents":null`) {
		t.Error("agents must not marshal as null")
	}
	if strings.Contains(string(raw), `"secrets":null`) {
		t.Error("secrets must not marshal as null")
	}
}

func sortedKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
