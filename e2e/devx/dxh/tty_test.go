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

	start := func() *dxh.TTY {
		tty, err := dxh.StartTTY(ctx, t.TempDir(), "", "sh", "-c", "echo super-secret-token; read x")
		if err != nil {
			t.Fatalf("StartTTY: %v", err)
		}
		tty.Redact("super-secret-token")
		if err := tty.Expect(ctx, "super-secret-token"); err != nil {
			t.Fatalf("Expect: %v", err)
		}
		if err := tty.Send("\n"); err != nil {
			t.Fatalf("Send: %v", err)
		}
		if code, err := tty.Wait(ctx); err != nil || code != 0 {
			t.Fatalf("Wait = %d, %v; want 0", code, err)
		}
		return tty
	}

	start()
	tty2 := start()

	err := tty2.Expect(ctx, "this-text-never-appears")
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
