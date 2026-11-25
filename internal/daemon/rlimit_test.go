package daemon

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestCoreLimitZero(t *testing.T) {
	_ = Run(context.Background(), Options{
		ConfigPath: filepath.Join(t.TempDir(), "missing.yaml"),
		SecretsDir: t.TempDir(),
		SecretUID:  os.Getuid(),
		StateDir:   t.TempDir(),
		Version:    "test",
		Log:        &strings.Builder{},
	})
	var lim syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_CORE, &lim); err != nil {
		t.Fatalf("Getrlimit: %v", err)
	}
	if lim.Cur != 0 || lim.Max != 0 {
		t.Fatalf("RLIMIT_CORE = {Cur: %d, Max: %d}; want {0, 0}", lim.Cur, lim.Max)
	}
}
