package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kanthorlabs/sag/internal/credential"
	"github.com/kanthorlabs/sag/internal/podman"
	"github.com/kanthorlabs/sag/internal/secret"
	"github.com/kanthorlabs/sag/internal/service"
)

func fakeRunnerCfg() service.Config {
	return service.Config{
		Podman: podman.Runner{Path: "/dev/null"},
		CfgDir: "/tmp/fakecfg",
	}
}

func withFakeNoLockCfg(t *testing.T, cfg service.Config) {
	t.Helper()
	old := buildCfgNoLockFn
	buildCfgNoLockFn = func() (service.Config, func(), error) {
		return cfg, func() {}, nil
	}
	t.Cleanup(func() { buildCfgNoLockFn = old })
}

func withFakeList(t *testing.T, entries []credential.Entry, err error) {
	t.Helper()
	old := runCredentialList
	runCredentialList = func(_ context.Context, _ credential.ListOptions) ([]credential.Entry, error) {
		return entries, err
	}
	t.Cleanup(func() { runCredentialList = old })
}

func withFakeInspect(t *testing.T, entry credential.Entry, err error) {
	t.Helper()
	old := runCredentialInspect
	runCredentialInspect = func(_ context.Context, _ string, _ credential.InspectOptions) (credential.Entry, error) {
		return entry, err
	}
	t.Cleanup(func() { runCredentialInspect = old })
}

func TestCredentialListJSONEnvelope(t *testing.T) {
	entries := []credential.Entry{
		{Name: "mydb", Kind: "postgres", TLS: "disable", Agents: []string{"agent-a"}},
	}
	withFakeNoLockCfg(t, fakeRunnerCfg())
	withFakeList(t, entries, nil)

	out := &bytes.Buffer{}
	std := IO{
		In:         strings.NewReader(""),
		Out:        out,
		Err:        &bytes.Buffer{},
		IsTerminal: func(any) bool { return false },
		Mode:       ModeHost,
	}

	code := credentialList([]string{"--json"}, std)
	if code != ExitOK {
		t.Fatalf("credentialList --json = %d; want %d", code, ExitOK)
	}

	var result struct {
		Credentials []map[string]any `json:"credentials"`
	}
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("parse JSON: %v\noutput: %s", err, out.String())
	}
	if len(result.Credentials) != 1 {
		t.Fatalf("want 1 credential, got %d", len(result.Credentials))
	}
	if result.Credentials[0]["name"] != "mydb" {
		t.Errorf("want name=mydb, got %v", result.Credentials[0]["name"])
	}
}

func TestCredentialListExitOnError(t *testing.T) {
	withFakeNoLockCfg(t, fakeRunnerCfg())
	withFakeList(t, nil, errors.New("storage unavailable"))

	std := IO{
		In:         strings.NewReader(""),
		Out:        &bytes.Buffer{},
		Err:        &bytes.Buffer{},
		IsTerminal: func(any) bool { return false },
		Mode:       ModeHost,
	}
	code := credentialList([]string{"--json"}, std)
	if code != ExitFailure {
		t.Fatalf("credentialList with error = %d; want %d", code, ExitFailure)
	}
}

func TestCredentialInspectJSONObject(t *testing.T) {
	entry := credential.Entry{
		Name: "mydb", Kind: "postgres", TLS: "disable", Agents: []string{"agent-a"},
	}
	withFakeNoLockCfg(t, fakeRunnerCfg())
	withFakeInspect(t, entry, nil)

	out := &bytes.Buffer{}
	std := IO{
		In:         strings.NewReader(""),
		Out:        out,
		Err:        &bytes.Buffer{},
		IsTerminal: func(any) bool { return false },
		Mode:       ModeHost,
	}

	code := credentialInspect([]string{"mydb", "--json"}, std)
	if code != ExitOK {
		t.Fatalf("credentialInspect --json = %d; want %d", code, ExitOK)
	}

	var result map[string]any
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("parse JSON: %v\noutput: %s", err, out.String())
	}
	if result["name"] != "mydb" {
		t.Errorf("want name=mydb, got %v", result["name"])
	}
}

func TestCredentialInspectNotFoundExitsFailure(t *testing.T) {
	withFakeNoLockCfg(t, fakeRunnerCfg())
	withFakeInspect(t, credential.Entry{}, fmt.Errorf("resource unknown not found: %w", credential.ErrResourceNotFound))

	errOut := &bytes.Buffer{}
	std := IO{
		In:         strings.NewReader(""),
		Out:        &bytes.Buffer{},
		Err:        errOut,
		IsTerminal: func(any) bool { return false },
		Mode:       ModeHost,
	}

	code := credentialInspect([]string{"unknown", "--json"}, std)
	if code != ExitFailure {
		t.Fatalf("credentialInspect not-found = %d; want %d", code, ExitFailure)
	}
	wantLine := "sag: resource unknown not found"
	if !strings.Contains(errOut.String(), wantLine) {
		t.Errorf("stderr = %q; want %q", errOut.String(), wantLine)
	}
}

func TestCredentialCreateMissingNameExitsUsage(t *testing.T) {
	std, _ := hostIO()
	code := credentialCreate([]string{}, std)
	if code != ExitUsage {
		t.Fatalf("credentialCreate (no args) = %d; want %d", code, ExitUsage)
	}
}

func TestCredentialCreateMissingKindExitsUsage(t *testing.T) {
	std, _ := hostIO()
	code := credentialCreate([]string{"mydb"}, std)
	if code != ExitUsage {
		t.Fatalf("credentialCreate (no --kind) = %d; want %d", code, ExitUsage)
	}
}

func TestCredentialCreateBadTLSExitsUsage(t *testing.T) {
	std, _ := hostIO()
	code := credentialCreate([]string{"mydb", "--kind", "postgres", "--tls", "badmode"}, std)
	if code != ExitUsage {
		t.Fatalf("credentialCreate (bad --tls) = %d; want %d", code, ExitUsage)
	}
}

func TestCredentialListAndInspectPassCAPem(t *testing.T) {
	withFakeNoLockCfg(t, fakeRunnerCfg())
	var listCA, inspectCA string
	oldList, oldInspect := runCredentialList, runCredentialInspect
	runCredentialList = func(_ context.Context, o credential.ListOptions) ([]credential.Entry, error) {
		listCA = o.CAPem
		return nil, nil
	}
	runCredentialInspect = func(_ context.Context, _ string, o credential.InspectOptions) (credential.Entry, error) {
		inspectCA = o.CAPem
		return credential.Entry{Name: "mydb"}, nil
	}
	t.Cleanup(func() { runCredentialList, runCredentialInspect = oldList, oldInspect })

	std := IO{
		In:         strings.NewReader(""),
		Out:        &bytes.Buffer{},
		Err:        &bytes.Buffer{},
		IsTerminal: func(any) bool { return false },
		Mode:       ModeHost,
	}
	credentialList(nil, std)
	credentialInspect([]string{"mydb"}, std)

	wantList := filepath.Join("/tmp/fakecfg", "ca.pem")
	if listCA != wantList {
		t.Errorf("list CAPem = %q; want %q", listCA, wantList)
	}
	if inspectCA != "" {
		t.Errorf("inspect CAPem = %q; want empty (B1: inspect does not pass CAPem)", inspectCA)
	}
}

func TestCredentialCreateDefaultTLSIsVerifyFull(t *testing.T) {
	opts, ok := parseCredentialCreateFlags([]string{"mydb", "--kind", "postgres"}, hostStd())
	if !ok {
		t.Fatal("parseCredentialCreateFlags returned false")
	}
	if string(opts.TLS) != "verify-full" {
		t.Errorf("default TLS = %q; want verify-full", opts.TLS)
	}
}

func TestCredentialCreateBackendFlag(t *testing.T) {
	cases := []struct {
		args []string
		ok   bool
		want string
	}{
		{nil, true, secret.BackendPodman},
		{[]string{"--backend", "podman"}, true, secret.BackendPodman},
		{[]string{"--backend", "aws-sm"}, true, secret.BackendAWSSM},
		{[]string{"--backend", "secret"}, false, ""},
	}
	for _, tc := range cases {
		opts, ok := parseCredentialCreateFlags(append([]string{"mydb", "--kind", "postgres"}, tc.args...), hostStd())
		if ok != tc.ok {
			t.Errorf("%v: ok = %v; want %v", tc.args, ok, tc.ok)
			continue
		}
		if ok && opts.Backend != tc.want {
			t.Errorf("%v: Backend = %q; want %q", tc.args, opts.Backend, tc.want)
		}
	}
}

func TestCredentialListDoesNotUseLock(t *testing.T) {
	lockCalled := false
	withFakeNoLockCfg(t, fakeRunnerCfg())

	oldLock := buildCfgFn
	buildCfgFn = func() (service.Config, func(), error) {
		lockCalled = true
		return fakeRunnerCfg(), func() {}, nil
	}
	t.Cleanup(func() { buildCfgFn = oldLock })

	withFakeList(t, nil, nil)

	std := IO{
		In:         strings.NewReader(""),
		Out:        &bytes.Buffer{},
		Err:        &bytes.Buffer{},
		IsTerminal: func(any) bool { return false },
		Mode:       ModeHost,
	}
	credentialList(nil, std)
	if lockCalled {
		t.Error("credentialList called the locking buildCfgFn; it must use buildCfgNoLockFn")
	}
}

func TestCredentialInspectDoesNotUseLock(t *testing.T) {
	lockCalled := false
	withFakeNoLockCfg(t, fakeRunnerCfg())

	oldLock := buildCfgFn
	buildCfgFn = func() (service.Config, func(), error) {
		lockCalled = true
		return fakeRunnerCfg(), func() {}, nil
	}
	t.Cleanup(func() { buildCfgFn = oldLock })

	withFakeInspect(t, credential.Entry{Name: "mydb"}, nil)

	std := IO{
		In:         strings.NewReader(""),
		Out:        &bytes.Buffer{},
		Err:        &bytes.Buffer{},
		IsTerminal: func(any) bool { return false },
		Mode:       ModeHost,
	}
	credentialInspect([]string{"mydb"}, std)
	if lockCalled {
		t.Error("credentialInspect called the locking buildCfgFn; it must use buildCfgNoLockFn")
	}
}

func TestCredentialListTextAllFields(t *testing.T) {
	ts := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	trueBool := true
	falseBool := false
	entries := []credential.Entry{
		{
			Name:        "mydb",
			Kind:        "postgres",
			Description: "my desc",
			TLS:         "verify-full",
			Agents:      []string{"claude-code", "pi"},
			Secrets: []credential.SecretEntry{
				{
					Name:      "sag-res-mydb.conn",
					Backend:   "podman",
					Role:      "connection",
					Exists:    &trueBool,
					Owned:     true,
					CreatedAt: &ts,
					Mounted:   &falseBool,
				},
			},
		},
	}
	withFakeNoLockCfg(t, fakeRunnerCfg())
	withFakeList(t, entries, nil)

	out := &bytes.Buffer{}
	std := IO{
		In:         strings.NewReader(""),
		Out:        out,
		Err:        &bytes.Buffer{},
		IsTerminal: func(any) bool { return false },
		Mode:       ModeHost,
	}
	code := credentialList(nil, std)
	if code != ExitOK {
		t.Fatalf("credentialList = %d; want 0", code)
	}

	got := out.String()
	fields := map[string]string{
		"name":                  "mydb",
		"kind":                  "postgres",
		"description":           "my desc",
		"tls":                   "verify-full",
		"agents":                "claude-code,pi",
		"secrets[0].name":       "sag-res-mydb.conn",
		"secrets[0].backend":    "podman",
		"secrets[0].role":       "connection",
		"secrets[0].exists":     "true",
		"secrets[0].owned":      "true",
		"secrets[0].created_at": "2024-01-15T10:30:00Z",
		"secrets[0].mounted":    "false",
	}
	for label, want := range fields {
		line := label + ": " + want
		if !strings.Contains(got, line) {
			t.Errorf("text output missing %q\nfull output:\n%s", line, got)
		}
	}
}

func TestCredentialInspectTextContainerRunning(t *testing.T) {
	trueBool := true
	entry := credential.Entry{
		Name:             "mydb",
		Kind:             "postgres",
		TLS:              "disable",
		Agents:           []string{"agent-a"},
		ContainerRunning: &trueBool,
	}
	withFakeNoLockCfg(t, fakeRunnerCfg())
	withFakeInspect(t, entry, nil)

	out := &bytes.Buffer{}
	std := IO{
		In:         strings.NewReader(""),
		Out:        out,
		Err:        &bytes.Buffer{},
		IsTerminal: func(any) bool { return false },
		Mode:       ModeHost,
	}
	code := credentialInspect([]string{"mydb"}, std)
	if code != ExitOK {
		t.Fatalf("credentialInspect = %d; want 0", code)
	}

	got := out.String()
	if !strings.Contains(got, "container_running: true") {
		t.Errorf("text output missing 'container_running: true'\nfull output:\n%s", got)
	}
}

func TestCredentialListTextNullFields(t *testing.T) {
	entries := []credential.Entry{
		{
			Name:    "mydb",
			Kind:    "postgres",
			TLS:     "disable",
			Agents:  []string{"agent-a"},
			Secrets: []credential.SecretEntry{{Name: "sag-res-mydb.conn", Backend: "podman", Role: "connection"}},
		},
	}
	withFakeNoLockCfg(t, fakeRunnerCfg())
	withFakeList(t, entries, nil)

	out := &bytes.Buffer{}
	std := IO{
		In:         strings.NewReader(""),
		Out:        out,
		Err:        &bytes.Buffer{},
		IsTerminal: func(any) bool { return false },
		Mode:       ModeHost,
	}
	credentialList(nil, std)

	got := out.String()
	for _, field := range []string{"secrets[0].exists: null", "secrets[0].created_at: null", "secrets[0].mounted: null"} {
		if !strings.Contains(got, field) {
			t.Errorf("text output missing %q for nil pointer field\nfull output:\n%s", field, got)
		}
	}
}

func hostStd() IO {
	return IO{
		In:         strings.NewReader(""),
		Out:        io.Discard,
		Err:        io.Discard,
		IsTerminal: func(any) bool { return false },
		Mode:       ModeHost,
	}
}
