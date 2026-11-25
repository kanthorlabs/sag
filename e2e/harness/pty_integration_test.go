//go:build integration

package harness

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"testing"
	"time"

	"github.com/kanthorlabs/sag/internal/testenv"
)

const ptyImage = "docker.io/library/busybox:1.37.0-musl"

func TestPTYDrivesPodmanExec(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	suffix := make([]byte, 4)
	if _, err := rand.Read(suffix); err != nil {
		t.Fatal(err)
	}
	name := "sag-e2e-pty-" + hex.EncodeToString(suffix)
	res, err := testenv.Podman(ctx, nil, "run", "-d", "--rm", "--name", name, ptyImage, "sleep", "300")
	if err != nil || res.ExitCode != 0 {
		t.Fatalf("start %s: %v %+v", name, err, res)
	}
	t.Cleanup(func() {
		_, _ = testenv.Podman(context.Background(), nil, "rm", "-f", "-t", "0", name)
	})

	tty, err := startTTY(ctx, "podman", "exec", "-it", name, "sh", "-c", `test -t 0 && test -t 1 && echo tty-ok; read x; echo "got-$x"; exit 7`)
	if err != nil {
		t.Fatal(err)
	}
	if err := tty.Expect(ctx, "tty-ok"); err != nil {
		t.Fatal(err)
	}
	if err := tty.Send("hello\n"); err != nil {
		t.Fatal(err)
	}
	if err := tty.Expect(ctx, "got-hello"); err != nil {
		t.Fatal(err)
	}
	out, err := tty.Wait(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if out.ExitCode != 7 {
		t.Errorf("exit code = %d, want 7; output %q", out.ExitCode, out.Stdout)
	}

	plain, err := testenv.Podman(ctx, nil, "exec", name, "sh", "-c", "test -t 1")
	if err != nil {
		t.Fatal(err)
	}
	if plain.ExitCode == 0 {
		t.Error("podman exec without -t has a terminal on stdout")
	}
}

func TestPTYExpectTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	tty, err := startTTY(ctx, "sh", "-c", "echo ready; sleep 30")
	if err != nil {
		t.Fatal(err)
	}
	short, cancelShort := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancelShort()
	if err := tty.Expect(short, "never"); err == nil {
		t.Fatal("Expect returned nil for absent text")
	}
	cancel()
	_, _ = tty.Wait(context.Background())
}
