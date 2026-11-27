//go:build devxe2e

package devx_test

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/kanthorlabs/sag/e2e/devx/dxh"
)

func init() {
	credentialTests["DXAC12_ConnectionCheck"] = DXAC12_ConnectionCheck
}

func DXAC12_ConnectionCheck(t *testing.T, ctx context.Context) {
	t.Helper()
	dxh.EnsureBootstrapped(t)

	uid := fmt.Sprintf("%d", os.Getuid())
	if err := waitLaunchAgentIdle(ctx, uid); err != nil {
		t.Fatalf("DXAC12: wait launch agent idle: %v", err)
	}

	closedPort := freePort(t)
	dsn12 := (&url.URL{
		Scheme: "postgres",
		User:   url.UserPassword("testuser", "testpass"),
		Host:   net.JoinHostPort("host.containers.internal", portString(closedPort)),
		Path:   "/testdb",
	}).String()

	t.Run("exit_1_no_secret", func(t *testing.T) {
		name := "dxac12a"
		code, combined := runSAGWithInput(t, ctx, []byte(dsn12),
			"credential", "create", name,
			"--kind", "postgres",
			"--agent", "claude-code",
			"--tls", "disable",
			"--stdin",
		)
		if code != 1 {
			t.Fatalf("DXAC12: create with closed port: exit %d; want 1; output: %s", code, combined)
		}
		if secretExists(ctx, t, "sag-res-"+name+".conn") {
			t.Fatalf("DXAC12: secret sag-res-%s.conn exists after failed create", name)
		}
	})

	t.Run("accept_failed_check", func(t *testing.T) {
		name := "dxac12b"
		code, combined := runSAGWithInput(t, ctx, []byte(dsn12),
			"credential", "create", name,
			"--kind", "postgres",
			"--agent", "claude-code",
			"--tls", "disable",
			"--accept-failed-check",
			"--stdin",
		)
		if code != 0 {
			t.Fatalf("DXAC12: create --accept-failed-check: exit %d; want 0; output: %s", code, combined)
		}
		warnCount := countWarnLines(combined)
		if warnCount != 1 {
			t.Fatalf("DXAC12: --accept-failed-check output has %d warn lines; want 1; output: %s", warnCount, combined)
		}
		if !secretExists(ctx, t, "sag-res-"+name+".conn") {
			t.Fatalf("DXAC12: secret sag-res-%s.conn absent after --accept-failed-check", name)
		}
	})
}

func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	return port
}

func portString(port int) string {
	return fmt.Sprintf("%d", port)
}

func runSAGWithInput(t *testing.T, ctx context.Context, stdin []byte, args ...string) (int, string) {
	t.Helper()
	bin, err := dxh.TestBinary()
	if err != nil {
		t.Fatalf("TestBinary: %v", err)
	}
	groupDir := t.TempDir()
	binDir := t.TempDir()
	env, err := dxh.ChildEnv(os.Environ(), groupDir)
	if err != nil {
		t.Fatalf("ChildEnv: %v", err)
	}
	env = append(env, "PATH="+dxh.TestPATH(binDir))

	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = env
	cmd.Stdin = bytes.NewReader(stdin)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	runErr := cmd.Run()
	code := 0
	if runErr != nil {
		if exitErr, ok := runErr.(*exec.ExitError); ok {
			code = exitErr.ExitCode()
		} else {
			t.Fatalf("runSAGWithInput(%v): %v", args, runErr)
		}
	}
	return code, out.String()
}

func countWarnLines(output string) int {
	count := 0
	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "warn ") {
			count++
		}
	}
	return count
}

func secretExists(ctx context.Context, t *testing.T, name string) bool {
	t.Helper()
	r := dxh.NewRunner()
	out, err := r.Output(ctx, "podman", "secret", "ls", "--filter", "name=^"+name+"$", "--format", "{{.Name}}")
	if err != nil {
		t.Fatalf("podman secret ls %s: %v", name, err)
	}
	return strings.TrimSpace(string(out)) == name
}
