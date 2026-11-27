package dxh

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

const (
	canaryResourceName = "canary"
	canaryPGAgent      = "pi"
	canaryPGContainer  = "sag-canary-pg"
	canaryPGImage      = "docker.io/library/postgres:17.12-bookworm"
	canaryPGPort       = "5432/tcp"
	canaryPGSecretName = "canary-pg-password"
	canaryReadyTimeout = 90 * time.Second
	CanaryPrompt       = "PostgreSQL DSN for canary (input hidden): "
)

var (
	ensureCanaryBootstrapFn = EnsureBootstrapped
	ensureCanaryCreateFn    = defaultCanaryCreate
)

func EnsureCanary(t *testing.T) {
	t.Helper()
	ensureCanaryBootstrapFn(t)

	home := os.Getenv("HOME")
	cfgDir := CfgDir(home)
	sagYAML := filepath.Join(cfgDir, "sag.yaml")

	if canaryExistsInConfig(sagYAML) {
		if ActiveCanaryParts == (CanaryParts{}) {
			t.Fatal("canary resource exists in sag.yaml but was not created by this process; re-run after sag teardown removes the canary")
		}
		return
	}

	r := newRunnerFn()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	if err := ensureCanaryCreateFn(ctx, r, t); err != nil {
		t.Fatalf("EnsureCanary: %v", err)
	}
}

func defaultCanaryCreate(ctx context.Context, r Runner, t *testing.T) error {
	t.Helper()

	if err := removeCanaryLeftovers(ctx, r); err != nil {
		return err
	}

	pwdHex, err := randomHex()
	if err != nil {
		return fmt.Errorf("generate pwd: %w", err)
	}

	if err := createCanaryPGSecret(ctx, pwdHex); err != nil {
		return err
	}

	suffix, err := randomHex()
	if err != nil {
		return fmt.Errorf("generate name suffix: %w", err)
	}
	user, database := "cu_"+suffix[:12], "cd_"+suffix[:12]

	if err := startCanaryPG(ctx, r, user, database); err != nil {
		return err
	}

	if err := waitCanaryPGReady(ctx, r, user); err != nil {
		return err
	}

	port, err := canaryPGHostPort(ctx, r)
	if err != nil {
		return err
	}

	dsn := buildCanaryDSN(user, pwdHex, port, database)

	parts, err := parseCanaryDSN(dsn)
	if err != nil {
		return fmt.Errorf("parse canary DSN: %w", err)
	}

	uid := strconv.Itoa(os.Getuid())
	if err := WaitLaunchAgentIdle(ctx, r, uid); err != nil {
		return fmt.Errorf("wait launch agent idle before canary create: %w", err)
	}

	if err := runCanaryCredentialCreate(ctx, t, dsn, parts); err != nil {
		return err
	}

	ActiveCanaryParts = parts
	return nil
}

func randomHex() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func createCanaryPGSecret(ctx context.Context, pwd string) error {
	cmd := exec.CommandContext(ctx, "podman", "secret", "create",
		"--label", "sag.devx-e2e=1",
		canaryPGSecretName, "-")
	cmd.Stdin = strings.NewReader(pwd)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("create pg pwd secret: %w\n%s", err, out)
	}
	return nil
}

func startCanaryPG(ctx context.Context, r Runner, user, database string) error {
	_, err := r.Output(ctx,
		"podman", "run", "--detach",
		"--label", "sag.devx-e2e=1",
		"--name", canaryPGContainer,
		"--publish", "127.0.0.1::5432",
		"--secret", canaryPGSecretName,
		"--env", "POSTGRES_PASSWORD_FILE=/run/secrets/"+canaryPGSecretName,
		"--env", "POSTGRES_USER="+user,
		"--env", "POSTGRES_DB="+database,
		canaryPGImage,
	)
	return err
}

func waitCanaryPGReady(ctx context.Context, r Runner, user string) error {
	deadline := time.Now().Add(canaryReadyTimeout)
	for {
		if _, err := r.Output(ctx, "podman", "exec", canaryPGContainer, "pg_isready", "-U", user, "-h", "127.0.0.1"); err == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("postgres %s not ready after %s", canaryPGContainer, canaryReadyTimeout)
		}
		time.Sleep(500 * time.Millisecond)
	}
}

func canaryPGHostPort(ctx context.Context, r Runner) (string, error) {
	out, err := r.Output(ctx, "podman", "port", canaryPGContainer, canaryPGPort)
	if err != nil {
		return "", fmt.Errorf("get port for %s: %w", canaryPGContainer, err)
	}
	line := strings.TrimSpace(string(out))
	if i := strings.LastIndex(line, ":"); i >= 0 {
		return line[i+1:], nil
	}
	return "", fmt.Errorf("unexpected port output: %q", line)
}

func buildCanaryDSN(user, pwd, port, database string) string {
	u := &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(user, pwd),
		Host:   "host.containers.internal:" + port,
		Path:   "/" + database,
	}
	return u.String()
}

func parseCanaryDSN(dsn string) (CanaryParts, error) {
	u, err := url.Parse(dsn)
	if err != nil {
		return CanaryParts{}, err
	}
	pwd, _ := u.User.Password()
	return CanaryParts{
		Password: pwd,
		Host:     u.Hostname(),
		User:     u.User.Username(),
		Database: strings.TrimPrefix(u.Path, "/"),
	}, nil
}

func runCanaryCredentialCreate(ctx context.Context, t *testing.T, dsn string, parts CanaryParts) error {
	t.Helper()
	bin, err := TestBinary()
	if err != nil {
		return err
	}
	groupDir := t.TempDir()
	binDir := t.TempDir()

	tty, err := StartTTY(ctx, groupDir, binDir, bin,
		"credential", "create", canaryResourceName,
		"--kind", "postgres",
		"--agent", canaryPGAgent,
		"--tls", "disable",
	)
	if err != nil {
		return fmt.Errorf("start TTY for canary create: %w", err)
	}

	tty.Redact(dsn, parts.Password, parts.Host, parts.User, parts.Database)

	if err := tty.Expect(ctx, CanaryPrompt); err != nil {
		return fmt.Errorf("canary create prompt: %w", err)
	}

	if err := tty.WaitEchoOff(ctx); err != nil {
		return fmt.Errorf("wait echo off before send: %w", err)
	}

	if err := tty.Send(dsn + "\n"); err != nil {
		return fmt.Errorf("send canary DSN: %w", err)
	}

	code, err := tty.Wait(ctx)
	transcript := tty.RawOutput()
	ActiveCanaryTranscript = []byte(transcript)

	if err != nil {
		return fmt.Errorf("canary create wait: %w", err)
	}
	if code != 0 {
		return fmt.Errorf("sag credential create canary: exit %d", code)
	}
	return nil
}

func canaryExistsInConfig(sagYAML string) bool {
	data, err := os.ReadFile(sagYAML)
	if err != nil {
		return false
	}
	return strings.Contains(string(data), canaryResourceName+":")
}

func removeCanaryLeftovers(ctx context.Context, r Runner) error {
	if _, err := r.Output(ctx, "podman", "rm", "--force", "--ignore", canaryPGContainer); err != nil {
		return fmt.Errorf("remove leftover %s: %w", canaryPGContainer, err)
	}
	out, err := r.Output(ctx, "podman", "secret", "ls", "--filter", "name=^"+canaryPGSecretName+"$", "--format", "{{.Name}}")
	if err != nil {
		return fmt.Errorf("list leftover %s: %w", canaryPGSecretName, err)
	}
	if strings.TrimSpace(string(out)) == "" {
		return nil
	}
	if _, err := r.Output(ctx, "podman", "secret", "rm", canaryPGSecretName); err != nil {
		return fmt.Errorf("remove leftover %s: %w", canaryPGSecretName, err)
	}
	return nil
}
