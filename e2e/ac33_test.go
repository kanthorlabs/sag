//go:build e2e

package e2e

import (
	"archive/tar"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kanthorlabs/sag/e2e/harness"
	"github.com/kanthorlabs/sag/internal/secret"
)

func init() {
	harness.Register(harness.AC{
		ID:    "AC33",
		Phase: harness.PhaseParallel,
		Run:   runAC33,
	})
}

var ac33ForbiddenBinaries = []string{"busybox", "sh", "bash", "dash", "ash", "toybox"}

var ac33RequiredSecrets = []string{secret.MasterKeyName, "sag-tls-cert", "sag-tls-key"}

func runAC33(t *testing.T, env harness.Env) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	t.Run("ac33-secret-mounts", func(t *testing.T) {
		res, err := env.Exec(ctx, harness.ContainerSAG, harness.BusyboxTarget, "sh", "-c", harness.BusyboxTarget+` stat -c "%n %u %a %F" `+secret.Dir+`/*`)
		if err != nil {
			t.Fatal(err)
		}
		if res.ExitCode != 0 {
			t.Fatalf("stat exit = %d, stdout %q, stderr %q", res.ExitCode, res.Stdout, res.Stderr)
		}
		seen := map[string]bool{}
		for _, line := range strings.Split(strings.TrimSpace(res.Stdout), "\n") {
			fields := strings.Fields(line)
			if len(fields) < 4 {
				t.Fatalf("stat line %q does not have four fields", line)
			}
			if kind := strings.Join(fields[3:], " "); kind != "regular file" {
				t.Logf("%s is a %s, not a secret file", fields[0], kind)
				continue
			}
			if fields[1] != "65532" || fields[2] != "400" {
				t.Errorf("%s: owner %s mode %s, want 65532 400", fields[0], fields[1], fields[2])
			}
			seen[path.Base(fields[0])] = true
		}
		for _, name := range ac33RequiredSecrets {
			if !seen[name] {
				t.Errorf("%s/%s is not in the stat output %q", secret.Dir, name, res.Stdout)
			}
		}
		want, err := ac33SecretCount(ctx, env)
		if err != nil {
			t.Fatal(err)
		}
		if len(seen) != want {
			t.Errorf("stat shows %d files, the container has %d secrets: %q", len(seen), want, res.Stdout)
		}
	})

	t.Run("ac33-no-busybox-in-image", func(t *testing.T) {
		suffix := make([]byte, 4)
		if _, err := rand.Read(suffix); err != nil {
			t.Fatal(err)
		}
		name := "sag-e2e-ac33-" + hex.EncodeToString(suffix)
		if res, err := env.Podman(ctx, "create", "--name", name, env.Image()); err != nil || res.ExitCode != 0 {
			t.Fatalf("podman create: %v %+v", err, res)
		}
		t.Cleanup(func() { env.Podman(context.Background(), "rm", "-f", "-t", "0", name) })
		res, err := env.Podman(ctx, "export", name)
		if err != nil || res.ExitCode != 0 {
			t.Fatalf("podman export: %v exit %d stderr %q", err, res.ExitCode, res.Stderr)
		}
		entries := 0
		tr := tar.NewReader(strings.NewReader(res.Stdout))
		for {
			h, err := tr.Next()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				t.Fatalf("read export: %v", err)
			}
			entries++
			if slices.Contains(ac33ForbiddenBinaries, path.Base(h.Name)) {
				t.Errorf("image %s holds %s", env.Image(), h.Name)
			}
		}
		if entries == 0 {
			t.Fatal("image export has no entries")
		}
	})

	t.Run("ac33-no-busybox-in-deploy", func(t *testing.T) {
		deploy := filepath.Join(env.RepoRoot(), "deploy")
		err := filepath.WalkDir(deploy, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			data, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			if strings.Contains(strings.ToLower(string(data)), "busybox") {
				t.Errorf("%s holds busybox", p)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	})
}

func ac33SecretCount(ctx context.Context, env harness.Env) (int, error) {
	res, err := env.Podman(ctx, "container", "inspect", "--format", "{{json .Config.Secrets}}", env.ContainerName(harness.ContainerSAG))
	if err != nil {
		return 0, err
	}
	if res.ExitCode != 0 {
		return 0, errors.New("podman container inspect: " + res.Stderr)
	}
	var secrets []json.RawMessage
	if err := json.Unmarshal([]byte(res.Stdout), &secrets); err != nil {
		return 0, err
	}
	return len(secrets), nil
}
