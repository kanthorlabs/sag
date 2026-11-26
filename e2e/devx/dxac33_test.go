//go:build devxe2e

package devx_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/kanthorlabs/sag/e2e/devx/dxh"
)

func worktreeRoot() (string, error) {
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func testDXAC33_ProductBinary(t *testing.T, ctx context.Context, groupDir, binDir string) {
	t.Run("DXAC33_ProductBinary", func(t *testing.T) {
		root, err := worktreeRoot()
		if err != nil {
			t.Fatalf("worktreeRoot: %v", err)
		}
		sagBin := filepath.Join(root, "bin", "sag")
		sagTestBin := filepath.Join(root, "bin", "sag-devxtest")

		for _, tc := range []struct {
			bin     string
			wantTag bool
		}{
			{sagBin, false},
			{sagTestBin, true},
		} {
			if _, err := os.Stat(tc.bin); err != nil {
				t.Fatalf("binary not found: %s: %v", tc.bin, err)
			}
			vmOut, err := exec.CommandContext(ctx, "go", "version", "-m", tc.bin).Output()
			if err != nil {
				t.Fatalf("go version -m %s: %v", tc.bin, err)
			}
			hasTag := strings.Contains(string(vmOut), "-tags=devxtest")
			if tc.wantTag && !hasTag {
				t.Errorf("build setting -tags=devxtest not found in %s", tc.bin)
			}
			if !tc.wantTag && hasTag {
				t.Errorf("build setting -tags=devxtest found in %s; want absent", tc.bin)
			}
		}

		lsOut, err := exec.CommandContext(ctx, "git", "ls-files").Output()
		if err != nil {
			t.Fatalf("git ls-files: %v", err)
		}
		for _, line := range strings.Split(strings.TrimSpace(string(lsOut)), "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			if strings.HasPrefix(line, "docs/") {
				continue
			}
			if !strings.HasSuffix(line, ".go") || strings.HasSuffix(line, "_test.go") {
				continue
			}
			raw, err := os.ReadFile(filepath.Join(root, line))
			if err != nil {
				t.Fatalf("read %s: %v", line, err)
			}
			if strings.Contains(string(raw), "--showsecret") {
				t.Errorf("non-test source %s contains --showsecret", line)
			}
		}

		testProductIgnoresTesthooks(t, ctx, groupDir, binDir, sagBin)
	})
}

func testProductIgnoresTesthooks(t *testing.T, ctx context.Context, groupDir, binDir, sagBin string) {
	t.Helper()
	env, err := dxh.ChildEnv(os.Environ(), groupDir)
	if err != nil {
		t.Fatalf("DXAC33: child env: %v", err)
	}
	env = append(env, "PATH="+dxh.TestPATH(binDir))
	env = append(env, "SAG_TEST_IMAGE=nonexistent-image-dxac33-should-not-use")
	env = append(env, "SAG_TEST_REGISTRY=nonexistent-registry.dxac33.example.com")

	cmd := exec.CommandContext(ctx, sagBin, "bootstrap",
		"--autostart", "no",
		"--agent-setup", "claude-code=no",
		"--agent-setup", "pi=no")
	cmd.Env = env
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("DXAC33: bin/sag bootstrap with SAG_TEST hooks: %v\n%s", err, out)
	}

	r := dxh.NewRunner()
	imgOut, err := r.Output(ctx, "podman", "container", "inspect", "sag", "--format", "{{.ImageName}}")
	if err != nil {
		t.Fatalf("DXAC33: podman container inspect sag: %v", err)
	}
	imageName := strings.TrimSpace(string(imgOut))
	if strings.Contains(imageName, "nonexistent") {
		t.Fatalf("DXAC33: container uses SAG_TEST_IMAGE %q; product must ignore SAG_TEST_* variables", imageName)
	}
}
