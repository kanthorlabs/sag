//go:build integration

package podman

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/kanthorlabs/sag/internal/testenv"
)

func TestContainerInspectListsSecrets(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	run, err := testenv.NewRun(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := run.Close(ctx); err != nil {
			t.Logf("run cleanup: %v", err)
		}
	}()

	secretName := fmt.Sprintf("sag-e2e-%s-tls", run.ID)
	if err := run.CreateSecret(ctx, secretName, []byte("test-cert-value")); err != nil {
		t.Fatal(err)
	}

	podmanPath := resolvePodman()
	if podmanPath == "" {
		t.Skip("podman not found")
	}

	containerName := fmt.Sprintf("sag-e2e-%s-insp", run.ID)
	res, err := testenv.Podman(ctx, nil,
		"run", "-d", "--rm",
		"--name", containerName,
		"--network", run.Network,
		"--secret", secretName,
		"docker.io/library/busybox:1.37.0-musl",
		"sh", "-c", "sleep 3600",
	)
	if err != nil {
		t.Fatal("podman run:", err)
	}
	if res.ExitCode != 0 {
		t.Fatalf("podman run exit %d: %s", res.ExitCode, res.Stderr)
	}
	t.Cleanup(func() {
		_, _ = testenv.Podman(context.Background(), nil, "stop", "--time", "0", containerName)
	})

	r := Runner{Path: podmanPath}
	info, err := r.ContainerInspect(ctx, containerName)
	if err != nil {
		t.Fatal("ContainerInspect:", err)
	}

	if info.Name == "" {
		t.Fatalf("container name empty")
	}

	t.Logf("container %s has %d secrets from podman container inspect", containerName, len(info.Secrets))
	for _, s := range info.Secrets {
		t.Logf("  secret: %s", s.Name)
	}

	found := false
	for _, s := range info.Secrets {
		if s.Name == secretName {
			found = true
		}
	}
	if !found {
		t.Fatalf("secret %q not in container inspect result; secrets: %v", secretName, info.Secrets)
	}
}

func resolvePodman() string {
	for _, p := range []string{"/usr/local/bin/podman", "/opt/homebrew/bin/podman", "/usr/bin/podman"} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	res, err := testenv.Podman(context.Background(), nil, "version")
	if err == nil && res.ExitCode == 0 {
		return "podman"
	}
	return ""
}
