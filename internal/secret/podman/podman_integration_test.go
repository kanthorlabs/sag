//go:build integration

package podman_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	ipodman "github.com/kanthorlabs/sag/internal/podman"
	"github.com/kanthorlabs/sag/internal/secret"
	spodman "github.com/kanthorlabs/sag/internal/secret/podman"
)

func resolveIntegPodman(t *testing.T) string {
	t.Helper()
	for _, p := range []string{"/usr/local/bin/podman", "/opt/homebrew/bin/podman", "/usr/bin/podman"} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	t.Skip("podman not found")
	return ""
}

func TestBackendIntegration(t *testing.T) {
	podmanPath := resolveIntegPodman(t)
	runner := ipodman.Runner{Path: podmanPath}
	b := spodman.New(runner)

	id := fmt.Sprintf("%d", time.Now().UnixNano())
	name1 := "sag-e2e-" + id + "-integ1"
	name2 := "sag-e2e-" + id + "-integ2"

	ctx := context.Background()

	t.Cleanup(func() {
		runner.SecretRemove(context.Background(), name1)
		runner.SecretRemove(context.Background(), name2)
	})

	cred := secret.NewCredential(name1, []byte("test-integration-value"))
	labels := map[string]string{"sag.devx-e2e": "1", "sag.resource": "test"}

	if err := b.Put(ctx, name1, cred, labels); err != nil {
		t.Fatalf("Put: %v", err)
	}

	err := b.Put(ctx, name1, cred, labels)
	if !errors.Is(err, secret.ErrExists) {
		t.Fatalf("second Put: want ErrExists, got %v", err)
	}

	meta, err := b.Metadata(ctx, name1)
	if err != nil {
		t.Fatalf("Metadata: %v", err)
	}
	if meta.Name != name1 {
		t.Errorf("Name = %q, want %q", meta.Name, name1)
	}
	if meta.Labels["sag.devx-e2e"] != "1" {
		t.Errorf("Labels = %v, want sag.devx-e2e=1", meta.Labels)
	}

	cred2 := secret.NewCredential(name2, []byte("test-integration-value-2"))
	if err := b.Put(ctx, name2, cred2, map[string]string{"sag.devx-e2e": "1"}); err != nil {
		t.Fatalf("Put name2: %v", err)
	}

	all, err := b.List(ctx, map[string]string{"sag.resource": "test"})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	found := false
	for _, m := range all {
		if m.Name == name1 {
			found = true
		}
	}
	if !found {
		t.Errorf("List did not return %q: %v", name1, all)
	}

	if err := b.Delete(ctx, name1); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	if err := b.Delete(ctx, name1); err != nil {
		t.Fatalf("second Delete (absent): %v", err)
	}

	_, err = b.Metadata(ctx, name1)
	if !errors.Is(err, secret.ErrNotFound) {
		t.Fatalf("Metadata after delete: want ErrNotFound, got %v", err)
	}

	if err := b.Delete(ctx, name2); err != nil {
		t.Fatalf("Delete name2: %v", err)
	}
}
