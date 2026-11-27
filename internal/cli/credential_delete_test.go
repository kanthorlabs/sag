package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/kanthorlabs/sag/internal/deploy"
	"github.com/kanthorlabs/sag/internal/podman"
	"github.com/kanthorlabs/sag/internal/secret"
	"github.com/kanthorlabs/sag/internal/service"
	"github.com/kanthorlabs/sag/internal/stepline"
)

const fakePodmanDeleteScript = `#!/bin/sh
d="$SAG_FAKE_PODMAN_LOG"
n=$(ls "$d" | grep -c '\.args$')
printf '%s\n' "$@" > "$d/$n.args"
env > "$d/$n.env"
cat > "$d/$n.stdin"
case "$1 $2" in
"container inspect")
	if [ "${SAG_FAKE_CONTAINER_RUNNING:-true}" = "true" ]; then
		printf 'true\n'
	else
		printf 'false\n'
	fi
	;;
"secret ls") printf 'sag-res-deltest.conn\n' ;;
"secret inspect") printf '[{"ID":"a","Spec":{"Name":"%s","Labels":{"sag.resource":"deltest"}},"CreatedAt":"2024-01-01T00:00:00Z","UpdatedAt":"2024-01-01T00:00:00Z"}]\n' "$3" ;;
"secret rm") exit 0 ;;
"exec sag") exit 0 ;;
"run --rm") exit 0 ;;
esac
exit 0
`

const deleteSAGYAML = `version: 1
server:
  tls_cert: secret:sag-tls-cert
  tls_key: secret:sag-tls-key
resources:
  deltest:
    kind: fakedb
    connection: secret:sag-res-deltest.conn
    agents: [agent-a]
`

func readDeleteCalls(t *testing.T, dir string) []fakePodmanCall {
	t.Helper()
	names, _ := filepath.Glob(filepath.Join(dir, "*.args"))
	sort.Strings(names)
	var calls []fakePodmanCall
	for _, a := range names {
		base := strings.TrimSuffix(a, ".args")
		read := func(p string) string {
			b, _ := os.ReadFile(p)
			return string(b)
		}
		calls = append(calls, fakePodmanCall{
			args:  read(a),
			env:   read(base + ".env"),
			stdin: read(base + ".stdin"),
		})
	}
	return calls
}

func callArgs(c fakePodmanCall) []string {
	var out []string
	for _, line := range strings.Split(strings.TrimSuffix(c.args, "\n"), "\n") {
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

func findCallByVerb(calls []fakePodmanCall, verb, noun string) (fakePodmanCall, bool) {
	for _, c := range calls {
		lines := strings.SplitN(c.args, "\n", 3)
		if len(lines) >= 2 && lines[0] == verb && lines[1] == noun {
			return c, true
		}
	}
	return fakePodmanCall{}, false
}

func checkNoCanaryOrJWT(t *testing.T, calls []fakePodmanCall) {
	t.Helper()
	for i, c := range calls {
		for j, arg := range callArgs(c) {
			if strings.Contains(arg, "canary") {
				t.Errorf("call %d arg %d contains 'canary': %q", i, j, arg)
			}
			if len(arg) > 7 && arg[:7] == "eyJhbGc" {
				t.Errorf("call %d arg %d looks like JWT: %q (truncated)", i, j, arg[:20])
			}
		}
	}
}

func withDeleteFakeTTY(t *testing.T, name string) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(w, name+"\n"); err != nil {
		t.Fatal(err)
	}
	w.Close()
	oldTTY := openTTY
	openTTY = func() (*os.File, error) { return r, nil }
	t.Cleanup(func() { openTTY = oldTTY })
}

func TestCredentialDeleteExitCodes(t *testing.T) {
	t.Run("exit2_flag", func(t *testing.T) {
		std := IO{Out: io.Discard, Err: &bytes.Buffer{}}
		code := credentialDelete([]string{"--yes", "myres"}, std)
		if code != ExitUsage {
			t.Errorf("want ExitUsage(2), got %d", code)
		}
	})

	t.Run("exit3_open_tty_fails", func(t *testing.T) {
		var buildCalled bool
		oldBuild := buildCfgFn
		buildCfgFn = func() (service.Config, func(), error) {
			buildCalled = true
			return service.Config{}, func() {}, nil
		}
		t.Cleanup(func() { buildCfgFn = oldBuild })

		oldTTY := openTTY
		openTTY = func() (*os.File, error) { return nil, errors.New("no tty") }
		t.Cleanup(func() { openTTY = oldTTY })

		std := IO{Out: io.Discard, Err: &bytes.Buffer{}}
		code := credentialDelete([]string{"myres"}, std)
		if code != ExitRefusal {
			t.Errorf("want ExitRefusal(3), got %d", code)
		}
		if buildCalled {
			t.Errorf("buildCfgFn called after openTTY failed")
		}
	})

	t.Run("exit1_build_cfg_fails", func(t *testing.T) {
		r, w, _ := os.Pipe()
		w.Close()
		oldTTY := openTTY
		openTTY = func() (*os.File, error) { return r, nil }
		t.Cleanup(func() { openTTY = oldTTY })

		withFakeCfgErr(t, errors.New("build failed"))

		std := IO{Out: io.Discard, Err: &bytes.Buffer{}}
		code := credentialDelete([]string{"myres"}, std)
		if code != ExitFailure {
			t.Errorf("want ExitFailure(1), got %d", code)
		}
	})

	t.Run("exit3_wrong_name", func(t *testing.T) {
		binDir, logDir, cfgDir := t.TempDir(), t.TempDir(), t.TempDir()
		fake := filepath.Join(binDir, "podman")
		if err := os.WriteFile(fake, []byte(fakePodmanDeleteScript), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(cfgDir, "sag.yaml"), []byte(deleteSAGYAML), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Setenv("SAG_FAKE_PODMAN_LOG", logDir)

		r, w, _ := os.Pipe()
		if _, err := io.WriteString(w, "WRONG\n"); err != nil {
			t.Fatal(err)
		}
		w.Close()
		oldTTY := openTTY
		openTTY = func() (*os.File, error) { return r, nil }
		t.Cleanup(func() { openTTY = oldTTY })

		withFakeCfg(t, service.Config{Podman: podman.Runner{Path: fake}, CfgDir: cfgDir})
		oldStart := startFn
		startFn = func(_ context.Context, _ service.Config, _ stepline.Writer) error { return nil }
		t.Cleanup(func() { startFn = oldStart })

		std := IO{Out: io.Discard, Err: &bytes.Buffer{}}
		code := credentialDelete([]string{"deltest"}, std)
		if code != ExitRefusal {
			t.Errorf("want ExitRefusal(3) for wrong name, got %d", code)
		}
	})
}

func TestDeleteChildProcessArgLists(t *testing.T) {
	for _, tc := range []struct {
		name    string
		running string
	}{
		{"running", "true"},
		{"stopped", "false"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			binDir, logDir, cfgDir := t.TempDir(), t.TempDir(), t.TempDir()
			fake := filepath.Join(binDir, "podman")
			if err := os.WriteFile(fake, []byte(fakePodmanDeleteScript), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(cfgDir, "sag.yaml"), []byte(deleteSAGYAML), 0o644); err != nil {
				t.Fatal(err)
			}
			t.Setenv("SAG_FAKE_PODMAN_LOG", logDir)
			t.Setenv("SAG_FAKE_CONTAINER_RUNNING", tc.running)

			withDeleteFakeTTY(t, "deltest")
			withFakeCfg(t, service.Config{
				Podman: podman.Runner{Path: fake},
				CfgDir: cfgDir,
				Image:  "sag:test",
			})
			oldStart := startFn
			startFn = func(_ context.Context, _ service.Config, _ stepline.Writer) error { return nil }
			t.Cleanup(func() { startFn = oldStart })

			std := IO{Out: io.Discard, Err: &bytes.Buffer{}}
			code := credentialDelete([]string{"deltest"}, std)
			if code != ExitOK {
				t.Fatalf("credentialDelete = %d; want ExitOK; stderr: %s",
					code, std.Err.(*bytes.Buffer).String())
			}

			calls := readDeleteCalls(t, logDir)
			if len(calls) == 0 {
				t.Fatal("no podman calls recorded")
			}

			p := deploy.DefaultFlags.Product
			masterKeySpec := p.Secret(secret.MasterKeyName, secret.MasterKeyName, "")
			stateVol := p.StateVolume(deploy.StateVolumeName)

			if tc.running == "true" {
				c, ok := findCallByVerb(calls, "exec", "sag")
				if !ok {
					t.Fatal("no exec call found")
				}
				wantExec := []string{"exec", "sag", "sag", "extension", "revoke", "deltest"}
				if got := callArgs(c); !reflect.DeepEqual(got, wantExec) {
					t.Errorf("exec args\ngot:  %v\nwant: %v", got, wantExec)
				}
			} else {
				c, ok := findCallByVerb(calls, "run", "--rm")
				if !ok {
					t.Fatal("no run --rm call found")
				}
				var wantRun []string
				wantRun = append(wantRun, "run", "--rm")
				wantRun = append(wantRun, p.SecurityArgs()...)
				wantRun = append(wantRun, "-v", stateVol)
				wantRun = append(wantRun, "--secret", masterKeySpec)
				wantRun = append(wantRun, "sag:test", "extension", "revoke", "deltest")
				if got := callArgs(c); !reflect.DeepEqual(got, wantRun) {
					t.Errorf("run args\ngot:  %v\nwant: %v", got, wantRun)
				}
			}

			if c, ok := findCallByVerb(calls, "secret", "ls"); ok {
				wantLs := []string{"secret", "ls", "--format", "{{.Name}}"}
				if got := callArgs(c); !reflect.DeepEqual(got, wantLs) {
					t.Errorf("secret ls args\ngot:  %v\nwant: %v", got, wantLs)
				}
			} else {
				t.Error("no secret ls call found")
			}

			if c, ok := findCallByVerb(calls, "secret", "inspect"); ok {
				wantInspect := []string{"secret", "inspect", "sag-res-deltest.conn"}
				if got := callArgs(c); !reflect.DeepEqual(got, wantInspect) {
					t.Errorf("secret inspect args\ngot:  %v\nwant: %v", got, wantInspect)
				}
			} else {
				t.Error("no secret inspect call found")
			}

			if c, ok := findCallByVerb(calls, "secret", "rm"); ok {
				wantRm := []string{"secret", "rm", "sag-res-deltest.conn"}
				if got := callArgs(c); !reflect.DeepEqual(got, wantRm) {
					t.Errorf("secret rm args\ngot:  %v\nwant: %v", got, wantRm)
				}
			} else {
				t.Error("no secret rm call found")
			}

			checkNoCanaryOrJWT(t, calls)
		})
	}
}
