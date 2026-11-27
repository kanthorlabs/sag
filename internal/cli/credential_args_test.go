package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/kanthorlabs/sag/internal/podman"
	"github.com/kanthorlabs/sag/internal/service"
	"github.com/kanthorlabs/sag/internal/stepline"
)

const fakePodmanScript = `#!/bin/sh
d="$SAG_FAKE_PODMAN_LOG"
n=$(ls "$d" | grep -c '\.args$')
printf '%s\n' "$@" > "$d/$n.args"
env > "$d/$n.env"
cat > "$d/$n.stdin"
case "$1 $2" in
"container inspect") echo true ;;
"secret inspect") echo "Error: no such secret" >&2; exit 125 ;;
esac
exit 0
`

type fakePodmanCall struct {
	args  string
	env   string
	stdin string
}

func readFakePodmanCalls(t *testing.T, dir string) []fakePodmanCall {
	t.Helper()
	names, err := filepath.Glob(filepath.Join(dir, "*.args"))
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(names)
	var calls []fakePodmanCall
	for _, a := range names {
		base := strings.TrimSuffix(a, ".args")
		read := func(p string) string {
			b, err := os.ReadFile(p)
			if err != nil {
				t.Fatal(err)
			}
			return string(b)
		}
		calls = append(calls, fakePodmanCall{args: read(a), env: read(base + ".env"), stdin: read(base + ".stdin")})
	}
	return calls
}

func TestCreateChildProcessesHoldNoValue(t *testing.T) {
	const value = "postgres://u_marker:pw_marker_7f3a@db.example.internal:5432/db_marker"

	binDir, logDir, cfgDir := t.TempDir(), t.TempDir(), t.TempDir()
	fake := filepath.Join(binDir, "podman")
	if err := os.WriteFile(fake, []byte(fakePodmanScript), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "sag.yaml"), []byte(baseSAGYAMLForArgs), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SAG_FAKE_PODMAN_LOG", logDir)

	withFakeCfg(t, service.Config{Podman: podman.Runner{Path: fake}, CfgDir: cfgDir})
	oldStart := startFn
	startFn = func(context.Context, service.Config, stepline.Writer) error { return nil }
	t.Cleanup(func() { startFn = oldStart })

	std := IO{
		In:         strings.NewReader(value + "\n"),
		Out:        &bytes.Buffer{},
		Err:        &bytes.Buffer{},
		IsTerminal: func(any) bool { return false },
		Mode:       ModeHost,
	}
	code := credentialCreate([]string{"argsdb", "--kind", "postgres", "--agent", "agent-a", "--tls", "disable", "--stdin"}, std)
	if code != ExitOK {
		t.Fatalf("credentialCreate = %d; want %d; stderr: %s", code, ExitOK, std.Err.(*bytes.Buffer).String())
	}

	calls := readFakePodmanCalls(t, logDir)
	if len(calls) == 0 {
		t.Fatal("no podman call recorded")
	}
	parts := []string{"pw_marker_7f3a", "u_marker", "db_marker", "db.example.internal"}
	var sawSecretCreate, sawExec bool
	for i, c := range calls {
		for _, p := range parts {
			if strings.Contains(c.args, p) {
				t.Errorf("call %d: an argument holds a part of the value", i)
			}
			if strings.Contains(c.env, p) {
				t.Errorf("call %d: the environment holds a part of the value", i)
			}
		}
		first := strings.SplitN(c.args, "\n", 3)
		switch {
		case len(first) >= 2 && first[0] == "secret" && first[1] == "create" && strings.Contains(c.args, "sag-res-argsdb.conn"):
			sawSecretCreate = true
			if !strings.Contains(c.stdin, "pw_marker_7f3a") {
				t.Error("secret create: the value is not on standard input")
			}
		case len(first) >= 2 && first[0] == "exec" && first[1] == "-i":
			sawExec = true
			if !strings.Contains(c.stdin, "pw_marker_7f3a") {
				t.Error("exec -i: the value is not on standard input")
			}
		default:
			if strings.Contains(c.stdin, "pw_marker_7f3a") {
				t.Errorf("call %d: standard input of an unexpected command holds the value", i)
			}
		}
	}
	if !sawSecretCreate || !sawExec {
		t.Fatalf("secret create seen=%v, exec -i seen=%v; want both", sawSecretCreate, sawExec)
	}
}

const baseSAGYAMLForArgs = `version: 1
server:
  tls_cert: secret:sag-tls-cert
  tls_key: secret:sag-tls-key
resources: {}
`
