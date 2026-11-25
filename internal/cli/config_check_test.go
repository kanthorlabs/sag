package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "github.com/kanthorlabs/sag/internal/adapter/postgres"
)

func configFixture(name string) string {
	return filepath.Join("..", "config", "testdata", name+".yaml")
}

func setConfigCheckPaths(t *testing.T, p Paths) {
	t.Helper()
	saved := paths
	paths = p
	t.Cleanup(func() { paths = saved })
}

func runConfigCheck(args ...string) (int, string, string) {
	out, errOut := &bytes.Buffer{}, &bytes.Buffer{}
	std := IO{In: strings.NewReader(""), Out: out, Err: errOut, IsTerminal: func(any) bool { return false }}
	code := Run(append([]string{"config", "check"}, args...), std)
	return code, out.String(), errOut.String()
}

func TestAC01_ConfigCheckRejects(t *testing.T) {
	setConfigCheckPaths(t, Paths{Config: configFixture("ac01-valid"), Secrets: filepath.Join(t.TempDir(), "absent"), SecretUID: os.Getuid()})
	for _, name := range []string{"ac01-inline-dsn", "ac01-unknown-key", "ac01-max-rows", "ac01-recheck-5s"} {
		t.Run(name, func(t *testing.T) {
			code, out, errOut := runConfigCheck("--config", configFixture(name))
			if code != ExitFailure {
				t.Fatalf("exit = %d; want %d; stderr %q", code, ExitFailure, errOut)
			}
			if strings.Contains(out+errOut, "postgres://") || strings.Contains(out+errOut, "p4ss") {
				t.Fatal("output holds the inline DSN")
			}
		})
	}
}

func TestAC01_ValidWithoutSecretsDir(t *testing.T) {
	setConfigCheckPaths(t, Paths{Config: configFixture("ac01-valid"), Secrets: filepath.Join(t.TempDir(), "absent"), SecretUID: os.Getuid()})
	if code, _, errOut := runConfigCheck("--config", configFixture("ac01-valid")); code != ExitOK {
		t.Fatalf("exit = %d; want %d; stderr %q", code, ExitOK, errOut)
	}
	if code, _, errOut := runConfigCheck(); code != ExitOK {
		t.Fatalf("default path: exit = %d; want %d; stderr %q", code, ExitOK, errOut)
	}
}

func TestConfigCheckSecretsDirMissingSecret(t *testing.T) {
	setConfigCheckPaths(t, Paths{Secrets: t.TempDir(), SecretUID: os.Getuid()})
	if code, _, _ := runConfigCheck("--config", configFixture("ac01-valid")); code != ExitFailure {
		t.Fatalf("exit = %d; want %d", code, ExitFailure)
	}
}

func TestConfigCheckUnknownKind(t *testing.T) {
	data, err := os.ReadFile(configFixture("ac01-valid"))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "sag.yaml")
	if err := os.WriteFile(path, bytes.Replace(data, []byte("kind: postgres"), []byte("kind: mysql"), 1), 0o600); err != nil {
		t.Fatal(err)
	}
	setConfigCheckPaths(t, Paths{Secrets: filepath.Join(t.TempDir(), "absent"), SecretUID: os.Getuid()})
	code, _, errOut := runConfigCheck("--config", path)
	if code != ExitFailure || !strings.Contains(errOut, "mysql") {
		t.Fatalf("exit = %d, stderr %q; want %d and the kind", code, errOut, ExitFailure)
	}
}

func TestConfigCheckUsage(t *testing.T) {
	for _, args := range [][]string{{"extra"}, {"--config"}, {"--nope"}} {
		if code, _, _ := runConfigCheck(args...); code != ExitUsage {
			t.Fatalf("args %q: exit = %d; want %d", args, code, ExitUsage)
		}
	}
}
