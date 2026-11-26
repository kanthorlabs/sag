package dxh

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func childEnv(groupDir, binDir string) ([]string, error) {
	env, err := ChildEnv(os.Environ(), groupDir)
	if err != nil {
		return nil, err
	}
	env = append(env, "PATH="+TestPATH(binDir))
	return env, nil
}

func SuiteEntry(t *testing.T, r Runner, pcfg PreconditionConfig, tcfg TeardownConfig, markerFile string) func() {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	if err := CheckRunLock(filepath.Dir(filepath.Dir(markerFile))); err != nil {
		t.Fatalf("suite entry: %v", err)
	}

	if _, err := os.Stat(markerFile); err == nil {
		pcfg.Marker = true
	}

	if err := CheckPreconditionsWithRecovery(ctx, r, pcfg, func(ctx context.Context) error {
		return Teardown(ctx, r, tcfg)
	}); err != nil {
		t.Fatalf("suite precondition: %v", err)
	}

	if err := os.MkdirAll(filepath.Dir(markerFile), 0o700); err != nil {
		t.Fatalf("suite entry mkdir: %v", err)
	}
	if err := os.WriteFile(markerFile, nil, 0o600); err != nil {
		t.Fatalf("suite entry write marker: %v", err)
	}

	return func() {
		_ = os.Remove(markerFile)
	}
}

func RunGroup(t *testing.T, ctx context.Context, r Runner, tcfg TeardownConfig, name string, setup func() error, subtests func(*testing.T)) {
	t.Helper()
	if err := Teardown(ctx, r, tcfg); err != nil {
		t.Fatalf("group %s pre-teardown: %v", name, err)
	}
	if setup != nil {
		if err := setup(); err != nil {
			t.Fatalf("group %s setup: %v", name, err)
		}
	}
	t.Run(name, subtests)
	if err := Teardown(ctx, r, tcfg); err != nil {
		t.Logf("group %s post-teardown: %v", name, err)
	}
}
