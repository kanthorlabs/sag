//go:build devxe2e

package devx_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/kanthorlabs/sag/e2e/devx/dxh"
)

func buildFakeInstall(t *testing.T, binDir string) {
	t.Helper()
	sagBin, err := dxh.TestBinary()
	if err != nil {
		t.Fatalf("buildFakeInstall: %v", err)
	}
	root := filepath.Dir(filepath.Dir(sagBin))
	out := filepath.Join(binDir, "fakeinstall")
	cmd := exec.Command("go", "build", "-o", out, "./e2e/devx/cmd/fakeinstall/")
	cmd.Dir = root
	cmd.Env = os.Environ()
	if data, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("buildFakeInstall: %v\n%s", err, data)
	}
}

func runBootstrapNoTerminal(t *testing.T, ctx context.Context, sagBin string, childEnv []string, extraArgs ...string) (stdout, stderr string, code int) {
	t.Helper()
	testCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	args := []string{"bootstrap", "--autostart", "no", "--agent-setup", "claude-code=no", "--agent-setup", "pi=no"}
	args = append(args, extraArgs...)

	cmd := exec.CommandContext(testCtx, sagBin, args...)
	cmd.Env = childEnv
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	var stdoutBuf, stderrBuf strings.Builder
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	runErr := cmd.Run()
	if runErr != nil {
		if exitErr, ok := runErr.(*exec.ExitError); ok {
			code = exitErr.ExitCode()
		} else {
			t.Fatalf("runBootstrapNoTerminal: %v", runErr)
		}
	}
	return stdoutBuf.String(), stderrBuf.String(), code
}

func DXAC07_ReleaseChecksum(t *testing.T, ctx context.Context) {
	t.Helper()

	content := []byte("fake-pkg-content-for-dxac07")
	h := sha256.Sum256(content)
	correctHash := hex.EncodeToString(h[:])

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write(content)
	}))
	defer srv.Close()

	pkgURL := srv.URL + "/test.pkg"

	groupDir := t.TempDir()
	binDir := t.TempDir()
	buildFakeInstall(t, binDir)

	sagBin, err := dxh.TestBinary()
	if err != nil {
		t.Fatalf("DXAC07: test binary: %v", err)
	}

	recordFile := filepath.Join(t.TempDir(), "fakeinstall.record")
	downloadTmpDir := t.TempDir()

	t.Setenv("SAG_TEST_ABSENT", "1")
	t.Setenv("SAG_TEST_FAKEINSTALL_RECORD", recordFile)
	t.Setenv("TMPDIR", downloadTmpDir)

	wrongBytes := sha256.Sum256([]byte("wrong-content-for-dxac07"))
	wrongHash := hex.EncodeToString(wrongBytes[:])
	t.Setenv("SAG_TEST_PINS", fmt.Sprintf("testpkg 0.1.0 darwin/arm64 release %s %s\n", pkgURL, wrongHash))

	childEnv, err := dxh.ChildEnv(os.Environ(), groupDir)
	if err != nil {
		t.Fatalf("DXAC07: child env: %v", err)
	}
	childEnv = append(childEnv, "PATH="+dxh.TestPATH(binDir))

	stdout, stderr, code := runBootstrapNoTerminal(t, ctx, sagBin, childEnv, "--auto-approve", "testpkg@0.1.0")
	if code != 1 {
		t.Fatalf("DXAC07 wrong-sha: exit code = %d; want 1\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}
	if _, err := os.Stat(recordFile); err == nil {
		t.Fatalf("DXAC07 wrong-sha: fakeinstall was called but should not have been after checksum mismatch")
	}
	pkgFiles, _ := filepath.Glob(filepath.Join(downloadTmpDir, "sag-deps-*", "*.pkg"))
	if len(pkgFiles) > 0 {
		t.Fatalf("DXAC07 wrong-sha: downloaded file not deleted: %v", pkgFiles)
	}
	if !strings.Contains(stderr, "checksum mismatch") {
		t.Fatalf("DXAC07 wrong-sha: stderr = %q; want 'checksum mismatch'", stderr)
	}

	_ = os.Remove(recordFile)
	t.Setenv("SAG_TEST_PINS", fmt.Sprintf("testpkg 0.1.0 darwin/arm64 release %s %s\n", pkgURL, correctHash))

	childEnv, err = dxh.ChildEnv(os.Environ(), groupDir)
	if err != nil {
		t.Fatalf("DXAC07: child env correct-sha: %v", err)
	}
	childEnv = append(childEnv, "PATH="+dxh.TestPATH(binDir))

	stdout, stderr, code = runBootstrapNoTerminal(t, ctx, sagBin, childEnv, "--auto-approve", "testpkg@0.1.0")
	if code != 0 {
		t.Fatalf("DXAC07 correct-sha: exit code = %d; want 0\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}

	recordData, err := os.ReadFile(recordFile)
	if err != nil {
		t.Fatalf("DXAC07 correct-sha: fakeinstall record not found: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(recordData)), "\n")
	if len(lines) != 1 {
		t.Fatalf("DXAC07 correct-sha: fakeinstall record has %d lines; want 1\n%s", len(lines), recordData)
	}
	parts := strings.SplitN(lines[0], "\t", 2)
	if len(parts) != 2 {
		t.Fatalf("DXAC07 correct-sha: fakeinstall record line malformed: %q", lines[0])
	}
	recordedPath := parts[0]
	recordedHash := parts[1]

	if filepath.Base(recordedPath) != "test.pkg" {
		t.Fatalf("DXAC07 correct-sha: fakeinstall got path %q; want basename test.pkg", recordedPath)
	}
	parentDir := filepath.Dir(recordedPath)
	if !strings.HasPrefix(filepath.Base(parentDir), "sag-deps-") || filepath.Dir(parentDir) != downloadTmpDir {
		t.Fatalf("DXAC07 correct-sha: fakeinstall path %q not under TMPDIR/sag-deps-*", recordedPath)
	}
	if recordedHash != correctHash {
		t.Fatalf("DXAC07 correct-sha: fakeinstall recorded hash %q; want %q", recordedHash, correctHash)
	}

	if !strings.Contains(stdout, "install testpkg") {
		t.Fatalf("DXAC07 correct-sha: stdout = %q; want 'install testpkg' line", stdout)
	}
}
