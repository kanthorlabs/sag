package credential

import (
	"context"
	"strings"
	"testing"

	"github.com/kanthorlabs/sag/internal/podman"
)

func TestOneShotRunsCorrectArgs(t *testing.T) {
	cmd := &fakePodmanCmd{}
	err := revokeOneShot(context.Background(), "myres", cmd, "sag:test")
	if err != nil {
		t.Fatalf("revokeOneShot: %v", err)
	}
	if len(cmd.args) == 0 {
		t.Fatal("no podman run call")
	}
	a := cmd.args[0]
	joined := strings.Join(a, " ")
	if a[0] != "run" {
		t.Errorf("first arg = %q; want run", a[0])
	}
	if !strings.Contains(joined, "sag-state") {
		t.Errorf("missing sag-state volume; args = %v", a)
	}
	if !strings.Contains(joined, "sag-master-key") {
		t.Errorf("missing sag-master-key secret; args = %v", a)
	}
	if !strings.Contains(joined, "myres") {
		t.Errorf("missing resource name; args = %v", a)
	}
	if !strings.Contains(joined, "extension") {
		t.Errorf("missing extension sub-command; args = %v", a)
	}
}

func TestOneShotPropagatesError(t *testing.T) {
	cmd := &fakePodmanCmd{execErr: &podman.RunError{ExitCode: 1, Stderr: "some error"}}
	err := revokeOneShot(context.Background(), "myres", cmd, "sag:test")
	if err == nil {
		t.Fatal("want error, got nil")
	}
}
