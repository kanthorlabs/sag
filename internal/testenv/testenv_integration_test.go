//go:build integration

package testenv

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

var shared *Postgres

func TestMain(m *testing.M) {
	os.Exit(runMain(m))
}

func runMain(m *testing.M) int {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	major, err := MajorFromEnv()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	shared, err = StartPostgres(ctx, major)
	if err != nil {
		fmt.Fprintln(os.Stderr, "start postgres:", err)
		return 1
	}
	defer shared.Close(context.Background())
	return m.Run()
}

func testContext(t *testing.T) context.Context {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	t.Cleanup(cancel)
	return ctx
}

func TestStartPostgresTLS(t *testing.T) {
	ctx := testContext(t)
	conn, err := shared.Connect(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	var ssl bool
	if err := conn.QueryRow(ctx, "SELECT ssl FROM pg_stat_ssl WHERE pid = pg_backend_pid()").Scan(&ssl); err != nil {
		t.Fatal(err)
	}
	if !ssl {
		t.Fatal("pg_stat_ssl.ssl is false")
	}
	var version string
	if err := conn.QueryRow(ctx, "SHOW server_version_num").Scan(&version); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(version, fmt.Sprint(shared.Major)) {
		t.Fatalf("server_version_num %s, want major %d", version, shared.Major)
	}
}

func TestSuperuser(t *testing.T) {
	ctx := testContext(t)
	conn, err := shared.Connect(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	var user string
	var super bool
	if err := conn.QueryRow(ctx, "SELECT current_user::text, rolsuper FROM pg_roles WHERE rolname = current_user").Scan(&user, &super); err != nil {
		t.Fatal(err)
	}
	if user != "postgres" || !super {
		t.Fatalf("user %q superuser %v, want postgres superuser", user, super)
	}
}

func TestLogStatementAll(t *testing.T) {
	ctx := testContext(t)
	since := time.Now().Add(-time.Second)
	conn, err := shared.Connect(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	marker := "testenv_marker_" + markerHex(t)
	if _, err := conn.Exec(ctx, "SELECT '"+marker+"'"); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		logs, err := shared.Logs(ctx, since)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(logs, marker) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("server log has no statement with %s", marker)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func TestTeardownRemovesSecrets(t *testing.T) {
	ctx := testContext(t)
	t.Setenv(KeepEnv, "")
	run, err := NewRun(ctx)
	if err != nil {
		t.Fatal(err)
	}
	pg, err := run.StartPostgres(ctx, shared.Major)
	if err != nil {
		run.Close(ctx)
		t.Fatal(err)
	}
	if err := run.CreateSecret(ctx, "extra-"+run.ID, []byte("value")); err != nil {
		run.Close(ctx)
		t.Fatal(err)
	}
	if !strings.Contains(podmanList(ctx, t, "{{.Name}}", "secret", "ls"), run.ID) {
		run.Close(ctx)
		t.Fatal("run secrets are absent before teardown")
	}
	if err := run.Close(ctx); err != nil {
		t.Fatal(err)
	}
	if out := podmanList(ctx, t, "{{.Name}}", "secret", "ls"); strings.Contains(out, run.ID) {
		t.Errorf("secrets of run %s remain: %s", run.ID, out)
	}
	if out := podmanList(ctx, t, "{{.Names}}", "ps", "-a"); strings.Contains(out, pg.Container) {
		t.Errorf("container %s remains", pg.Container)
	}
	if out := podmanList(ctx, t, "{{.Name}}", "network", "ls"); strings.Contains(out, run.Network) {
		t.Errorf("network %s remains", run.Network)
	}
}

func TestPG13Starts(t *testing.T) {
	ctx := testContext(t)
	pg, err := StartPostgres(ctx, 13)
	if err != nil {
		t.Fatal(err)
	}
	defer pg.Close(ctx)
	conn, err := pg.Connect(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	var version string
	if err := conn.QueryRow(ctx, "SHOW server_version_num").Scan(&version); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(version, "13") {
		t.Fatalf("server_version_num %s, want 13", version)
	}
}

func TestPodmanExitCode(t *testing.T) {
	ctx := testContext(t)
	res, err := Podman(ctx, nil, "secret", "inspect", "sag-e2e-absent-"+markerHex(t))
	if err != nil {
		t.Fatal(err)
	}
	if res.ExitCode == 0 {
		t.Fatal("exit code 0 for a missing secret")
	}
}

func podmanList(ctx context.Context, t *testing.T, format string, args ...string) string {
	t.Helper()
	res, err := Podman(ctx, nil, append(args, "--format", format)...)
	if err != nil || res.ExitCode != 0 {
		t.Fatalf("podman %v: %v %s", args, err, res.Stderr)
	}
	return res.Stdout
}

func markerHex(t *testing.T) string {
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(b)
}
