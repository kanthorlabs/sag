//go:build integration

package harness

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kanthorlabs/sag/internal/testenv"
)

func TestBusyboxExtracted(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	dir := t.TempDir()
	path, err := BusyboxMount(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(path) || filepath.Dir(path) != dir {
		t.Fatalf("path = %q, want an absolute path in %s", path, dir)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm() != 0o755 {
		t.Errorf("mode = %v, want a regular file with 0755", info.Mode())
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(data, []byte("\x7fELF")) {
		t.Error("file is not an ELF binary")
	}
	res, err := testenv.Podman(ctx, nil, "container", "exists", busyboxContainerName(dir))
	if err != nil {
		t.Fatal(err)
	}
	if res.ExitCode == 0 {
		t.Errorf("temporary container %s exists after return", busyboxContainerName(dir))
	}
}

func TestMountReadOnly(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	path, err := BusyboxMount(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	suffix := make([]byte, 4)
	if _, err := rand.Read(suffix); err != nil {
		t.Fatal(err)
	}
	name := "sag-e2e-busybox-ro-" + hex.EncodeToString(suffix)
	t.Cleanup(func() {
		_, _ = testenv.Podman(context.Background(), nil, "rm", "-f", "-t", "0", name)
	})
	if _, err := podman(ctx, runArgs(runSpec{Name: name, Busybox: path, Image: BusyboxImage, Command: []string{"sleep", "300"}})...); err != nil {
		t.Fatal(err)
	}

	res, err := podman(ctx, "container", "inspect", "--format", "{{json .Mounts}}", name)
	if err != nil {
		t.Fatal(err)
	}
	var mounts []struct {
		Destination string
		RW          bool
	}
	if err := json.Unmarshal([]byte(res.Stdout), &mounts); err != nil {
		t.Fatalf("decode mounts %q: %v", res.Stdout, err)
	}
	found := false
	for _, m := range mounts {
		if m.Destination == BusyboxTarget {
			found = true
			if m.RW {
				t.Errorf("mount %s is read-write", BusyboxTarget)
			}
		}
	}
	if !found {
		t.Fatalf("no mount at %s in %s", BusyboxTarget, res.Stdout)
	}

	run, err := podman(ctx, "exec", name, BusyboxTarget, "stat", "-c", "%a", BusyboxTarget)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(run.Stdout); got != "755" {
		t.Errorf("mode in container = %q, want 755", got)
	}
	write, err := testenv.Podman(ctx, nil, "exec", "--user", "0", name, BusyboxTarget, "sh", "-c", "echo x >> "+BusyboxTarget)
	if err != nil {
		t.Fatal(err)
	}
	if write.ExitCode == 0 {
		t.Error("write to the busybox mount succeeded")
	}
}
