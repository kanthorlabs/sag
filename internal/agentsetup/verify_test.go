package agentsetup

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fakeHarnessBin(t *testing.T, name, script string) string {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, name)
	content := "#!/bin/sh\n" + script + "\n"
	if err := os.WriteFile(bin, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestClaudeChildEnv(t *testing.T) {
	var capturedEnv []string
	origRun := runVerifyChild
	runVerifyChild = func(_ context.Context, name string, args []string, env []string, dir string) ([]byte, error) {
		capturedEnv = env
		return []byte("sag: https://localhost:8443/mcp (HTTP) - ✔ Connected\n"), nil
	}
	defer func() { runVerifyChild = origRun }()

	var out bytes.Buffer
	SetOut(&out)
	defer SetOut(os.Stdout)

	binDir := fakeHarnessBin(t, "claude", `echo "sag: Connected"`)
	opts := Options{
		Harness:    HarnessClaudeCode,
		Sub:        "claude-code",
		URL:        "https://localhost:8443/mcp",
		CfgDir:     t.TempDir(),
		PodmanPath: "podman",
		Env: map[string]string{
			"CLAUDE_CONFIG_DIR":   t.TempDir(),
			"NODE_EXTRA_CA_CERTS": "/some/other/path",
			"PATH":                binDir + ":/usr/bin:/bin",
		},
		Home: t.TempDir(),
	}
	_ = verify(context.Background(), opts)

	for _, e := range capturedEnv {
		k, v, _ := cutEnv(e)
		if k == "NODE_EXTRA_CA_CERTS" && v != "" {
			t.Errorf("Claude child env has NODE_EXTRA_CA_CERTS=%q; want it absent", v)
		}
	}
}

func TestPiChildEnv(t *testing.T) {
	var capturedEnv []string
	origRun := runVerifyChild
	runVerifyChild = func(_ context.Context, name string, args []string, env []string, dir string) ([]byte, error) {
		capturedEnv = env
		result := piMCPListOutput{
			Servers: []struct {
				Name  string `json:"name"`
				State string `json:"state"`
			}{
				{Name: "sag", State: "connected"},
			},
		}
		data, _ := json.Marshal(result)
		return data, nil
	}
	defer func() { runVerifyChild = origRun }()

	var out bytes.Buffer
	SetOut(&out)
	defer SetOut(os.Stdout)

	cfgDir := t.TempDir()
	binDir := fakeHarnessBin(t, "pi", `echo '{}'`)
	opts := Options{
		Harness:    HarnessPi,
		Sub:        "pi",
		URL:        "https://localhost:8443/mcp",
		CfgDir:     cfgDir,
		PodmanPath: "podman",
		Env: map[string]string{
			"PI_CODING_AGENT_DIR": t.TempDir(),
			"PATH":                binDir + ":/usr/bin:/bin",
		},
		Home: t.TempDir(),
	}
	_ = verify(context.Background(), opts)

	caPath := caFilePath(opts)
	found := false
	for _, e := range capturedEnv {
		k, v, _ := cutEnv(e)
		if k == "NODE_EXTRA_CA_CERTS" && v == caPath {
			found = true
		}
	}
	if !found {
		t.Errorf("pi child env missing NODE_EXTRA_CA_CERTS=%s; env=%v", caPath, capturedEnv)
	}
}

func TestNotConnectedExit1(t *testing.T) {
	origRun := runVerifyChild
	runVerifyChild = func(_ context.Context, name string, args []string, env []string, dir string) ([]byte, error) {
		return []byte("other: something\n"), nil
	}
	defer func() { runVerifyChild = origRun }()

	var out bytes.Buffer
	SetOut(&out)
	defer SetOut(os.Stdout)

	binDir := fakeHarnessBin(t, "claude", `echo "other: something"`)
	opts := Options{
		Harness:    HarnessClaudeCode,
		Sub:        "claude-code",
		URL:        "https://localhost:8443/mcp",
		CfgDir:     t.TempDir(),
		PodmanPath: "podman",
		Env: map[string]string{
			"CLAUDE_CONFIG_DIR": t.TempDir(),
			"PATH":              binDir + ":/usr/bin:/bin",
		},
		Home: t.TempDir(),
	}
	err := verify(context.Background(), opts)
	if err == nil {
		t.Error("verify returned nil; want error when sag not in output")
	}
	if !strings.Contains(out.String(), "fail") {
		t.Errorf("output %q: missing 'fail' line", out.String())
	}
}

func cutEnv(e string) (string, string, bool) {
	for i, c := range e {
		if c == '=' {
			return e[:i], e[i+1:], true
		}
	}
	return e, "", false
}
