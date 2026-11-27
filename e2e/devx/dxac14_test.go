//go:build devxe2e

package devx_test

import (
	"bytes"
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"

	"github.com/kanthorlabs/sag/e2e/devx/dxh"
)

func init() {
	credentialTests["DXAC14_CreateStdin"] = func(t *testing.T, ctx context.Context) {
		DXAC14_CreateStdin(t, ctx)
	}
}

func DXAC14_CreateStdin(t *testing.T, ctx context.Context) {
	t.Helper()
	dxh.EnsureCanary(t)

	uid := fmt.Sprintf("%d", os.Getuid())
	if err := dxh.WaitLaunchAgentIdle(ctx, dxh.NewRunner(), uid); err != nil {
		t.Fatalf("DXAC14: wait launch agent idle: %v", err)
	}

	r := dxh.NewRunner()
	port, err := canaryPGPort(ctx, r)
	if err != nil {
		t.Fatalf("DXAC14: get canary PG port: %v", err)
	}

	canary := dxh.ActiveCanaryParts
	validDSN := func(name string) string {
		u := &url.URL{
			Scheme: "postgres",
			User:   url.UserPassword(canary.User, canary.Password),
			Host:   "host.containers.internal:" + port,
			Path:   "/" + canary.Database,
		}
		_ = name
		return u.String()
	}

	t.Run("stdin_exit_0", func(t *testing.T) {
		code, _ := runSAGWithInput(t, ctx, []byte(validDSN("mydb-14a")),
			"credential", "create", "mydb-14a",
			"--kind", "postgres",
			"--agent", "pi",
			"--stdin",
			"--tls", "disable",
		)
		if code != 0 {
			t.Fatalf("DXAC14: create with stdin: exit %d; want 0", code)
		}
		listOut, err := runSAG(t, ctx, "credential", "list")
		if err != nil {
			t.Fatalf("DXAC14: credential list: %v", err)
		}
		if !strings.Contains(string(listOut), "mydb-14a") {
			t.Fatalf("DXAC14: mydb-14a not in credential list:\n%s", listOut)
		}
	})

	t.Run("oversized_input", func(t *testing.T) {
		oversized := make([]byte, 64*1024+1)
		for i := range oversized {
			oversized[i] = 'x'
		}
		code, combined := runSAGWithInput(t, ctx, oversized,
			"credential", "create", "mydb-14b",
			"--kind", "postgres",
			"--agent", "pi",
			"--stdin",
			"--tls", "disable",
		)
		if code != 1 {
			t.Fatalf("DXAC14: oversized input: exit %d; want 1", code)
		}
		if !strings.Contains(combined, "exceeds 64 KiB") {
			t.Fatalf("DXAC14: oversized output does not mention '64 KiB'")
		}
	})

	t.Run("nul_byte", func(t *testing.T) {
		nulInput := append([]byte(validDSN("mydb-14c")), '\x00', 'r', 'e', 's', 't')
		code, combined := runSAGWithInput(t, ctx, nulInput,
			"credential", "create", "mydb-14c",
			"--kind", "postgres",
			"--agent", "pi",
			"--stdin",
			"--tls", "disable",
		)
		if code != 1 {
			t.Fatalf("DXAC14: NUL input: exit %d; want 1", code)
		}
		if !strings.Contains(combined, "NUL") {
			t.Fatalf("DXAC14: NUL output does not mention 'NUL'")
		}
	})

	t.Run("piped_without_stdin_flag", func(t *testing.T) {
		code, _ := runSAGSetsid(t, ctx, []byte(validDSN("mydb-14d")),
			"credential", "create", "mydb-14d",
			"--kind", "postgres",
			"--agent", "pi",
			"--tls", "disable",
		)
		if code != 3 {
			t.Fatalf("DXAC14: piped without --stdin (setsid): exit %d; want 3", code)
		}
	})
}

func canaryPGPort(ctx context.Context, r dxh.Runner) (string, error) {
	out, err := r.Output(ctx, "podman", "port", "sag-canary-pg", "5432/tcp")
	if err != nil {
		return "", fmt.Errorf("podman port sag-canary-pg 5432/tcp: %w", err)
	}
	line := strings.TrimSpace(string(out))
	if i := strings.LastIndex(line, ":"); i >= 0 {
		return line[i+1:], nil
	}
	return "", fmt.Errorf("unexpected port output: %q", line)
}

func runSAGSetsid(t *testing.T, ctx context.Context, stdin []byte, args ...string) (int, string) {
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
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	runErr := cmd.Run()
	code := 0
	if runErr != nil {
		if exitErr, ok := runErr.(*exec.ExitError); ok {
			code = exitErr.ExitCode()
		} else {
			t.Fatalf("runSAGSetsid(%v): %v", args, runErr)
		}
	}
	return code, out.String()
}
