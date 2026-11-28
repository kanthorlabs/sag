//go:build devxe2e

package devx_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/kanthorlabs/sag/e2e/devx/dxh"
	"github.com/kanthorlabs/sag/internal/testenv"
)

func init() {
	credentialTests["DXAC37_CreateTLSCA"] = func(t *testing.T, ctx context.Context) {
		DXAC37_CreateTLSCA(t, ctx)
	}
}

const (
	pg37Container = "sag-pg37-tls"
	pg37Image     = "docker.io/library/postgres:17.12-bookworm"
	pg37Host      = "host.containers.internal"
	pg37SecretDir = "/run/secrets/"
	pg37PwdName   = "sag-pg37-cred"
	pg37CertName  = "sag-pg37-srv-crt"
	pg37KeyName   = "sag-pg37-srv-key"
)

func DXAC37_CreateTLSCA(t *testing.T, ctx context.Context) {
	t.Helper()
	dxh.EnsureCanary(t)

	uid := fmt.Sprintf("%d", os.Getuid())
	if err := dxh.WaitLaunchAgentIdle(ctx, dxh.NewRunner(), uid); err != nil {
		t.Fatalf("DXAC37: wait launch agent idle: %v", err)
	}

	ca, err := testenv.NewCA()
	if err != nil {
		t.Fatalf("DXAC37: new CA: %v", err)
	}

	certPEM, keyPEM, err := ca.Issue(pg37Host, "localhost")
	if err != nil {
		t.Fatalf("DXAC37: issue cert: %v", err)
	}

	buf := make([]byte, 16)
	if _, err = rand.Read(buf); err != nil {
		t.Fatalf("DXAC37: random cred: %v", err)
	}
	cred := hex.EncodeToString(buf)

	r := dxh.NewRunner()

	for _, pair := range []struct {
		name  string
		value []byte
	}{
		{pg37PwdName, []byte(cred)},
		{pg37CertName, certPEM},
		{pg37KeyName, keyPEM},
	} {
		cmd := exec.CommandContext(ctx, "podman", "secret", "create",
			"--label", "sag.devx-e2e=1",
			pair.name, "-")
		cmd.Stdin = bytes.NewReader(pair.value)
		if out, err2 := cmd.CombinedOutput(); err2 != nil {
			t.Fatalf("DXAC37: create secret %s: %v\n%s", pair.name, err2, out)
		}
	}

	pg37Addr, err := dxh.PublishAddr(ctx, r)
	if err != nil {
		t.Fatalf("DXAC37: %v", err)
	}
	_, err = r.Output(ctx,
		"podman", "run", "--detach",
		"--label", "sag.devx-e2e=1",
		"--name", pg37Container,
		"--publish", pg37Addr+"::5432",
		"--secret", pg37PwdName,
		"--secret", pg37CertName,
		"--secret", pg37KeyName+",uid=999,gid=999,mode=0600",
		"--env", "POSTGRES_PASSWORD_FILE="+pg37SecretDir+pg37PwdName,
		pg37Image,
		"-c", "ssl=on",
		"-c", "ssl_cert_file="+pg37SecretDir+pg37CertName,
		"-c", "ssl_key_file="+pg37SecretDir+pg37KeyName,
	)
	if err != nil {
		t.Fatalf("DXAC37: start postgres: %v", err)
	}

	deadline := time.Now().Add(90 * time.Second)
	for {
		if _, e := r.Output(ctx, "podman", "exec", pg37Container, "pg_isready", "-U", "postgres"); e == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("DXAC37: postgres not ready after 90s")
		}
		time.Sleep(500 * time.Millisecond)
	}

	portOut, err := r.Output(ctx, "podman", "port", pg37Container, "5432/tcp")
	if err != nil {
		t.Fatalf("DXAC37: get port: %v", err)
	}
	portStr := dxac37ParsePort(strings.TrimSpace(string(portOut)))
	portNum, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatalf("DXAC37: parse port %q: %v", portStr, err)
	}

	dsn := (&url.URL{
		Scheme: "postgres",
		User:   url.UserPassword("postgres", cred),
		Host:   net.JoinHostPort(pg37Host, strconv.Itoa(portNum)),
		Path:   "/postgres",
	}).String()

	caFile := filepath.Join(t.TempDir(), "ca.pem")
	if err = os.WriteFile(caFile, ca.CertPEM, 0o644); err != nil {
		t.Fatalf("DXAC37: write CA file: %v", err)
	}

	t.Run("create_ok", func(t *testing.T) {
		code, _ := runSAGWithInput(t, ctx, []byte(dsn),
			"credential", "create", "mydb-37",
			"--kind", "postgres",
			"--agent", "pi",
			"--stdin",
			"--tls", "verify-full",
			"--tls-ca", caFile,
		)
		if code != 0 {
			t.Fatalf("DXAC37: create with TLS CA: exit %d; want 0", code)
		}

		listOut, listErr := runSAG(t, ctx, "credential", "list")
		if listErr != nil {
			t.Fatalf("DXAC37: credential list: %v", listErr)
		}
		if !strings.Contains(string(listOut), "mydb-37") {
			t.Fatalf("DXAC37: mydb-37 not in credential list:\n%s", listOut)
		}

		testR := dxh.NewRunner()
		testOut, testErr := testR.Output(ctx, "podman", "exec", "sag", "sag", "resource", "test", "mydb-37")
		if testErr != nil {
			t.Fatalf("DXAC37: sag resource test mydb-37: %v\n%s", testErr, testOut)
		}

		caLabel := ac37SecretLabel(ctx, t, "sag-res-mydb-37.ca", "sag.role")
		if caLabel != "tls_ca" {
			t.Fatalf("DXAC37: CA secret sag.role label = %q; want %q", caLabel, "tls_ca")
		}
	})

	t.Run("hook_both_secrets_absent", func(t *testing.T) {
		code, combined := runSAGWithInputAndEnv(t, ctx, []byte(dsn),
			[]string{"SAG_TEST_FAIL_TLS_CA_CREATE=1"},
			"credential", "create", "mydb-37h",
			"--kind", "postgres",
			"--agent", "pi",
			"--stdin",
			"--tls", "verify-full",
			"--tls-ca", caFile,
		)
		if code != 1 {
			t.Fatalf("DXAC37: hook create: exit %d; want 1", code)
		}
		if !strings.Contains(combined, "test hook: fail at step 3 (tls-ca)") {
			t.Fatalf("DXAC37: hook output does not contain hook message")
		}
		if secretExists(ctx, t, "sag-res-mydb-37h.conn") {
			t.Fatalf("DXAC37: hook left sag-res-mydb-37h.conn behind")
		}
		if secretExists(ctx, t, "sag-res-mydb-37h.ca") {
			t.Fatalf("DXAC37: hook left sag-res-mydb-37h.ca behind")
		}
	})
}

func dxac37ParsePort(line string) string {
	if i := strings.LastIndex(line, ":"); i >= 0 {
		return line[i+1:]
	}
	return line
}

func ac37SecretLabel(ctx context.Context, t *testing.T, name, label string) string {
	t.Helper()
	r := dxh.NewRunner()
	out, err := r.Output(ctx, "podman", "secret", "inspect", name,
		"--format", fmt.Sprintf(`{{index .Spec.Labels "%s"}}`, label))
	if err != nil {
		t.Fatalf("secret inspect %s label %s: %v", name, label, err)
	}
	return strings.TrimSpace(string(out))
}
