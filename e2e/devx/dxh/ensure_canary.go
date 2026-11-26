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
	"strings"
	"testing"
	"time"
)

const (
	canaryResourceName = "canary"
	canaryPGAgent      = "pi"
	canaryPGContainer  = "sag-canary-pg"
	canaryPGSecretName = "canary-pg-password"
	canaryPGImage      = "docker.io/library/postgres:17.12-bookworm"
	canaryPGPort       = "5432/tcp"
	canaryReadyTimeout = 90 * time.Second
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

	password, err := randomCanaryPassword()
	if err != nil {
		return fmt.Errorf("generate password: %w", err)
	}

	if err := createCanaryPGSecret(ctx, password); err != nil {
		return err
	}

	if err := startCanaryPG(ctx, r); err != nil {
		return err
	}

	if err := waitCanaryPGReady(ctx, r); err != nil {
		return err
	}

	port, err := canaryPGHostPort(ctx, r)
	if err != nil {
		return err
	}

	dsn := buildCanaryDSN(password, port)

	parts, err := parseCanaryDSN(dsn)
	if err != nil {
		return fmt.Errorf("parse canary DSN: %w", err)
	}

	if err := runCanaryCredentialCreate(ctx, t, dsn); err != nil {
		return err
	}

	ActiveCanaryParts = parts
	return nil
}

func randomCanaryPassword() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func createCanaryPGSecret(ctx context.Context, password string) error {
	cmd := exec.CommandContext(ctx, "podman", "secret", "create",
		"--label", "sag.devx-e2e=1",
		canaryPGSecretName, "-")
	cmd.Stdin = strings.NewReader(password)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("create pg password secret: %w\n%s", err, out)
	}
	return nil
}

func startCanaryPG(ctx context.Context, r Runner) error {
	_, err := r.Output(ctx,
		"podman", "run", "--detach",
		"--label", "sag.devx-e2e=1",
		"--name", canaryPGContainer,
		"--publish", "127.0.0.1::5432",
		"--secret", canaryPGSecretName,
		"--env", "POSTGRES_PASSWORD_FILE=/run/secrets/"+canaryPGSecretName,
		canaryPGImage,
	)
	return err
}

func waitCanaryPGReady(ctx context.Context, r Runner) error {
	deadline := time.Now().Add(canaryReadyTimeout)
	for {
		if _, err := r.Output(ctx, "podman", "exec", canaryPGContainer, "pg_isready", "-U", "postgres"); err == nil {
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

func buildCanaryDSN(password, port string) string {
	return fmt.Sprintf("postgresql://postgres:%s@host.containers.internal:%s/postgres", password, port)
}

func parseCanaryDSN(dsn string) (CanaryParts, error) {
	u, err := url.Parse(dsn)
	if err != nil {
		return CanaryParts{}, err
	}
	var password string
	if u.User != nil {
		password, _ = u.User.Password()
	}
	return CanaryParts{
		Password: password,
		Host:     u.Hostname(),
		User:     u.User.Username(),
		Database: strings.TrimPrefix(u.Path, "/"),
	}, nil
}

func runCanaryCredentialCreate(ctx context.Context, t *testing.T, dsn string) error {
	t.Helper()
	bin, err := TestBinary()
	if err != nil {
		return err
	}
	groupDir := t.TempDir()
	binDir := t.TempDir()
	env, err := ChildEnv(os.Environ(), groupDir)
	if err != nil {
		return err
	}
	env = append(env, "PATH="+TestPATH(binDir))

	cmd := exec.CommandContext(ctx, bin,
		"credential", "create", canaryResourceName,
		"--kind", "postgres",
		"--agent", canaryPGAgent,
		"--stdin",
		"--tls", "disable",
	)
	cmd.Env = env
	cmd.Stdin = strings.NewReader(dsn)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("sag credential create canary: %w\n%s", err, out)
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
