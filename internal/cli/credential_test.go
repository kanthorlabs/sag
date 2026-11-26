package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/kanthorlabs/sag/internal/credential"
	"github.com/kanthorlabs/sag/internal/podman"
	"github.com/kanthorlabs/sag/internal/service"
)

func fakeRunnerCfg() service.Config {
	return service.Config{
		Podman: podman.Runner{Path: "/dev/null"},
		CfgDir: "/tmp/fakecfg",
	}
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
	withFakeCfg(t, fakeRunnerCfg())
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
	withFakeCfg(t, fakeRunnerCfg())
	withFakeList(t, nil, errors.New("storage unavailable"))

	std, _ := hostIO()
	code := credentialList([]string{"--json"}, std)
	if code != ExitFailure {
		t.Fatalf("credentialList with error = %d; want %d", code, ExitFailure)
	}
}

func TestCredentialInspectJSONObject(t *testing.T) {
	entry := credential.Entry{
		Name: "mydb", Kind: "postgres", TLS: "disable", Agents: []string{"agent-a"},
	}
	withFakeCfg(t, fakeRunnerCfg())
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
	withFakeCfg(t, fakeRunnerCfg())
	withFakeInspect(t, credential.Entry{}, errors.New("resource unknown not found"))

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
	if !strings.Contains(errOut.String(), "not found") {
		t.Errorf("stderr = %q; want 'not found'", errOut.String())
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
