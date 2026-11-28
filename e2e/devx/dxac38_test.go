//go:build devxe2e

package devx_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/kanthorlabs/sag/e2e/devx/dxh"
)

func init() {
	macTests["DXAC38_MacPkgChecksum"] = DXAC38_MacPkgChecksum
}

func DXAC38_MacPkgChecksum(t *testing.T, ctx context.Context) {
	t.Helper()

	sagBin, err := dxh.TestBinary()
	if err != nil {
		t.Fatalf("DXAC38: test binary: %v", err)
	}

	t.Run("wrong-hash", func(t *testing.T) {
		content := []byte("fake-podman-pkg-content-dxac38-wrong")
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write(content)
		}))
		defer srv.Close()

		wrongHash := strings.Repeat("0", 64)
		downloadDir := t.TempDir()
		groupDir := t.TempDir()
		binDir := t.TempDir()

		t.Setenv("SAG_TEST_ABSENT", "1")
		t.Setenv("SAG_TEST_PINS", fmt.Sprintf("podman 5.5.1 darwin/arm64 release %s/fake.pkg %s\n", srv.URL, wrongHash))

		childEnv, err := dxh.ChildEnv(os.Environ(), groupDir)
		if err != nil {
			t.Fatalf("DXAC38 wrong-hash: child env: %v", err)
		}
		childEnv = append(childEnv, "PATH="+dxh.TestPATH(binDir))
		childEnv = append(childEnv, "TMPDIR="+downloadDir)

		stdout, stderr, code := runPkgBootstrap(t, ctx, sagBin, childEnv)
		if code != 1 {
			t.Fatalf("DXAC38 wrong-hash: exit %d; want 1\nstdout=%s\nstderr=%s", code, stdout, stderr)
		}
		if !strings.Contains(stderr, "checksum mismatch") {
			t.Fatalf("DXAC38 wrong-hash: stderr = %q; want 'checksum mismatch'", stderr)
		}
		pkgFiles, _ := filepath.Glob(filepath.Join(downloadDir, "sag-deps-*", "*.pkg"))
		if len(pkgFiles) > 0 {
			t.Fatalf("DXAC38 wrong-hash: pkg file must be absent; found: %v", pkgFiles)
		}
	})

	t.Run("correct-hash", func(t *testing.T) {
		downloadDir := t.TempDir()
		groupDir := t.TempDir()
		binDir := t.TempDir()

		t.Setenv("SAG_TEST_ABSENT", "1")
		t.Setenv("SAG_TEST_STOP_BEFORE_INSTALL", "1")

		childEnv, err := dxh.ChildEnv(os.Environ(), groupDir)
		if err != nil {
			t.Fatalf("DXAC38 correct-hash: child env: %v", err)
		}
		childEnv = append(childEnv, "PATH="+dxh.TestPATH(binDir))
		childEnv = append(childEnv, "TMPDIR="+downloadDir)

		stdout, stderr, code := runPkgBootstrap(t, ctx, sagBin, childEnv)
		if code != 0 {
			t.Fatalf("DXAC38 correct-hash: exit %d; want 0\nstdout=%s\nstderr=%s", code, stdout, stderr)
		}

		var pkgPath string
		for _, line := range strings.Split(stdout, "\n") {
			line = strings.TrimSpace(line)
			if strings.HasSuffix(line, ".pkg") && filepath.IsAbs(line) {
				pkgPath = line
				break
			}
		}
		if pkgPath == "" {
			t.Fatalf("DXAC38 correct-hash: no .pkg path in stdout:\n%s", stdout)
		}
		if _, err := os.Stat(pkgPath); err == nil {
			t.Fatalf("DXAC38 correct-hash: pkg file still present at %s; must be cleaned up after verification", pkgPath)
		}
		pkgFiles, _ := filepath.Glob(filepath.Join(downloadDir, "sag-deps-*", "*.pkg"))
		if len(pkgFiles) > 0 {
			t.Fatalf("DXAC38 correct-hash: pkg files not cleaned up: %v", pkgFiles)
		}
	})
}

func runPkgBootstrap(t *testing.T, ctx context.Context, sagBin string, childEnv []string) (stdout, stderr string, code int) {
	t.Helper()
	args := []string{
		"bootstrap",
		"--autostart", "no",
		"--agent-setup", "claude-code=no",
		"--agent-setup", "pi=no",
		"--auto-approve", "podman@5.5.1",
	}
	cmd := exec.CommandContext(ctx, sagBin, args...)
	cmd.Env = childEnv
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	var stdoutBuf, stderrBuf strings.Builder
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	if runErr := cmd.Run(); runErr != nil {
		if exitErr, ok := runErr.(*exec.ExitError); ok {
			code = exitErr.ExitCode()
		} else {
			t.Fatalf("runPkgBootstrap: %v", runErr)
		}
	}
	return stdoutBuf.String(), stderrBuf.String(), code
}
