package dxh_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/kanthorlabs/sag/e2e/devx/dxh"
)

func TestExpectErrorRedacts(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	tty, err := dxh.StartTTY(ctx, t.TempDir(), "", "echo", "super-secret-token")
	if err != nil {
		t.Fatalf("StartTTY: %v", err)
	}
	tty.Redact("super-secret-token")

	if err := tty.Expect(ctx, "super-secret-token"); err != nil {
		t.Fatalf("Expect: %v", err)
	}
	_, _ = tty.Wait(ctx)

	tty2, err := dxh.StartTTY(ctx, t.TempDir(), "", "echo", "super-secret-token")
	if err != nil {
		t.Fatalf("StartTTY: %v", err)
	}
	tty2.Redact("super-secret-token")

	_, _ = tty2.Wait(ctx)

	err = tty2.Expect(ctx, "this-text-never-appears")
	if err == nil {
		t.Fatal("expected error from Expect, got nil")
	}
	if strings.Contains(err.Error(), "super-secret-token") {
		t.Errorf("error contains unredacted secret: %v", err)
	}
	if !strings.Contains(err.Error(), "[REDACTED]") {
		t.Errorf("error does not contain [REDACTED]: %v", err)
	}
}
