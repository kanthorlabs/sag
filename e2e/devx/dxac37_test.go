//go:build devxe2e

package devx_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
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

func DXAC37_CreateTLSCA(t *testing.T, ctx context.Context) {
	t.Helper()
	dxh.EnsureCanary(t)

	const (
		pgContainerName  = "sag-pg37-tls"
		pgPasswordSecret = "sag-pg37-password"
		pgCertSecret     = "sag-pg37-cert"
		pgKeySecret      = "sag-pg37-key"
		pgImage          = "docker.io/library/postgres:17.12-bookworm"
		hostName37       = "host.containers.internal"
		secretDir        = "/run/secrets/"
	)

	ca, err := testenv.NewCA()
	if err != nil {
		t.Fatalf("DXAC37: new CA: %v", err)
	}

	certPEM, keyPEM, err := ca.Issue(hostName37, "localhost")
	if err != nil {
		t.Fatalf("DXAC37: issue cert: %v", err)
	}

	buf := make([]byte, 16)
	if _, err = rand.Read(buf); err != nil {
		t.Fatalf("DXAC37: random password: %v", err)
	}
	password := hex.EncodeToString(buf)

	r := dxh.NewRunner()

	for _, pair := range []struct {
		name  string
		value []byte
	}{
		{pgPasswordSecret, []byte(password)},
		{pgCertSecret, certPEM},
		{pgKeySecret, keyPEM},
	} {
		cmd := exec.CommandContext(ctx, "podman", "secret", "create",
			"--label", "sag.devx-e2e=1",
			pair.name, "-")
		cmd.Stdin = bytes.NewReader(pair.value)
		if out, err2 := cmd.CombinedOutput(); err2 != nil {
			t.Fatalf("DXAC37: create secret %s: %v\n%s", pair.name, err2, out)
		}
	}

	_, err = r.Output(ctx,
		"podman", "run", "--detach",
		"--label", "sag.devx-e2e=1",
		"--name", pgContainerName,
		"--publish", "127.0.0.1::5432",
		"--secret", pgPasswordSecret,
		"--secret", pgCertSecret,
		"--secret", pgKeySecret+",uid=999,gid=999,mode=0600",
		"--env", "POSTGRES_PASSWORD_FILE="+secretDir+pgPasswordSecret,
		pgImage,
		"-c", "ssl=on",
		"-c", "ssl_cert_file="+secretDir+pgCertSecret,
		"-c", "ssl_key_file="+secretDir+pgKeySecret,
	)
	if err != nil {
		t.Fatalf("DXAC37: start postgres: %v", err)
	}

	deadline := time.Now().Add(90 * time.Second)
	for {
		if _, e := r.Output(ctx, "podman", "exec", pgContainerName, "pg_isready", "-U", "postgres"); e == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("DXAC37: postgres not ready after 90s")
		}
		time.Sleep(500 * time.Millisecond)
	}

	portOut, err := r.Output(ctx, "podman", "port", pgContainerName, "5432/tcp")
	if err != nil {
		t.Fatalf("DXAC37: get port: %v", err)
	}
	portStr := dxac37ParsePort(strings.TrimSpace(string(portOut)))
	portNum, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatalf("DXAC37: parse port %q: %v", portStr, err)
	}

	dsn := fmt.Sprintf("postgresql://postgres:%s@%s/%s",
		password,
		net.JoinHostPort(hostName37, strconv.Itoa(portNum)),
		"postgres",
	)

	caFile := filepath.Join(t.TempDir(), "ca.pem")
	if err = os.WriteFile(caFile, ca.CertPEM, 0o644); err != nil {
		t.Fatalf("DXAC37: write CA file: %v", err)
	}

	code, combined := runSAGWithInput(t, ctx, []byte(dsn),
		"credential", "create", "mydb-37",
		"--kind", "postgres",
		"--agent", "pi",
		"--stdin",
		"--tls", "verify-full",
		"--tls-ca", caFile,
	)
	if code != 0 {
		t.Fatalf("DXAC37: create with TLS CA: exit %d; want 0; output: %s", code, combined)
	}

	listOut, listErr := runSAG(t, ctx, "credential", "list")
	if listErr != nil {
		t.Fatalf("DXAC37: credential list: %v", listErr)
	}
	if !strings.Contains(string(listOut), "mydb-37") {
		t.Fatalf("DXAC37: mydb-37 not in credential list:\n%s", listOut)
	}
}

func dxac37ParsePort(line string) string {
	if i := strings.LastIndex(line, ":"); i >= 0 {
		return line[i+1:]
	}
	return line
}
