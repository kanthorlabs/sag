//go:build devxe2e

package devx_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/kanthorlabs/sag/e2e/devx/dxh"
)

func init() {
	agentTests["DXAC51_RemoteMode"] = func(t *testing.T, ctx context.Context) {
		DXAC51_RemoteMode(t, ctx)
	}
}

func DXAC51_RemoteMode(t *testing.T, ctx context.Context) {
	t.Helper()

	sagBin, err := dxh.TestBinary()
	if err != nil {
		t.Fatalf("DXAC51: test binary: %v", err)
	}

	fakeBin := dxac51BuildFakeharness(t, ctx)

	const (
		testURL = "https://127.0.0.1:8443/mcp"
		testSub = "remote-a"
	)

	t.Run("pty-exits-0-entry-no-podman", func(t *testing.T) {
		xdgDir, binDir := dxac51SetupDirs(t, fakeBin)
		groupDir := t.TempDir()
		podmanRecorder := dxac51FakePodman(t, binDir)

		jwtStr := dxac51JWT(testSub)

		script := fmt.Sprintf(
			"export XDG_CONFIG_HOME=%s; exec %s agent setup claude-code --url %s --sub %s --deny-rules no",
			xdgDir, sagBin, testURL, testSub,
		)
		tty, err := dxh.StartTTY(ctx, groupDir, binDir, "/bin/sh", "-c", script)
		if err != nil {
			t.Fatalf("DXAC51 pty: StartTTY: %v", err)
		}
		tty.Redact(jwtStr)

		if err := tty.Expect(ctx, "JWT for "+testSub+" (input hidden): "); err != nil {
			t.Fatalf("DXAC51 pty: expect JWT prompt: %v\noutput: %q", err, tty.RawOutput())
		}
		if err := tty.WaitEchoOff(ctx); err != nil {
			t.Fatalf("DXAC51 pty: echo off: %v", err)
		}
		if err := tty.Send(jwtStr + "\n"); err != nil {
			t.Fatalf("DXAC51 pty: send JWT: %v", err)
		}
		code, err := tty.Wait(ctx)
		if err != nil {
			t.Fatalf("DXAC51 pty: wait: %v", err)
		}
		if code != 0 {
			t.Fatalf("DXAC51 pty: exit %d; want 0\noutput: %q", code, tty.RawOutput())
		}

		mcpFile := filepath.Join(groupDir, "claude", ".claude.json")
		dxac51AssertMCPURL(t, mcpFile, testURL)

		if _, statErr := os.Stat(podmanRecorder); statErr == nil {
			t.Fatal("DXAC51 pty: podman was called in remote mode")
		}
	})

	t.Run("no-terminal-exits-3-no-change", func(t *testing.T) {
		xdgDir, binDir := dxac51SetupDirs(t, fakeBin)
		claudeDir := t.TempDir()

		snapBefore := dxac51SnapshotDirs(t, xdgDir, claudeDir)

		env := dxac51Env(xdgDir, claudeDir, binDir)
		cmd := exec.CommandContext(ctx, sagBin, "agent", "setup", "claude-code",
			"--url", testURL, "--sub", testSub, "--deny-rules", "no")
		cmd.Env = env
		cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
		var out bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &out
		runErr := cmd.Run()
		code := 0
		if exitErr, ok := runErr.(*exec.ExitError); ok {
			code = exitErr.ExitCode()
		} else if runErr != nil {
			t.Fatalf("DXAC51 no-terminal: run: %v", runErr)
		}
		if code != 3 {
			t.Fatalf("DXAC51 no-terminal: exit %d; want 3; output: %q", code, out.String())
		}

		snapAfter := dxac51SnapshotDirs(t, xdgDir, claudeDir)
		for path, before := range snapBefore {
			if after, ok := snapAfter[path]; !ok || before != after {
				t.Errorf("DXAC51 no-terminal: file changed: %s", path)
			}
		}
		for path := range snapAfter {
			if _, ok := snapBefore[path]; !ok {
				t.Errorf("DXAC51 no-terminal: new file created: %s", path)
			}
		}
	})

	t.Run("wrong-sub-exits-1", func(t *testing.T) {
		xdgDir, binDir := dxac51SetupDirs(t, fakeBin)

		otherSub := "other-sub"
		jwtStr := dxac51JWT(otherSub)

		script := fmt.Sprintf(
			"export XDG_CONFIG_HOME=%s; exec %s agent setup claude-code --url %s --sub %s --deny-rules no",
			xdgDir, sagBin, testURL, testSub,
		)
		groupDir := t.TempDir()
		tty, err := dxh.StartTTY(ctx, groupDir, binDir, "/bin/sh", "-c", script)
		if err != nil {
			t.Fatalf("DXAC51 wrong-sub: StartTTY: %v", err)
		}
		tty.Redact(jwtStr)

		if err := tty.Expect(ctx, "JWT for "+testSub+" (input hidden): "); err != nil {
			t.Fatalf("DXAC51 wrong-sub: expect prompt: %v\noutput: %q", err, tty.RawOutput())
		}
		if err := tty.WaitEchoOff(ctx); err != nil {
			t.Fatalf("DXAC51 wrong-sub: echo off: %v", err)
		}
		if err := tty.Send(jwtStr + "\n"); err != nil {
			t.Fatalf("DXAC51 wrong-sub: send JWT: %v", err)
		}
		code, err := tty.Wait(ctx)
		if err != nil {
			t.Fatalf("DXAC51 wrong-sub: wait: %v", err)
		}
		if code != 1 {
			t.Fatalf("DXAC51 wrong-sub: exit %d; want 1\noutput: %q", code, tty.RawOutput())
		}
	})

	t.Run("http-url-exits-2", func(t *testing.T) {
		xdgDir, binDir := dxac51SetupDirs(t, fakeBin)
		claudeDir := t.TempDir()
		env := dxac51Env(xdgDir, claudeDir, binDir)
		cmd := exec.CommandContext(ctx, sagBin, "agent", "setup", "claude-code",
			"--url", "http://127.0.0.1:8443/mcp", "--sub", testSub, "--deny-rules", "no")
		cmd.Env = env
		var out bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &out
		runErr := cmd.Run()
		code := 0
		if exitErr, ok := runErr.(*exec.ExitError); ok {
			code = exitErr.ExitCode()
		} else if runErr != nil {
			t.Fatalf("DXAC51 http-url: run: %v", runErr)
		}
		if code != 2 {
			t.Fatalf("DXAC51 http-url: exit %d; want 2; output: %q", code, out.String())
		}
	})

	t.Run("variable-header-exits-1-another-form", func(t *testing.T) {
		xdgDir, binDir := dxac51SetupDirs(t, fakeBin)
		claudeDir := t.TempDir()

		mcpFile := filepath.Join(claudeDir, ".claude.json")
		entry := map[string]any{
			"mcpServers": map[string]any{
				"sag": map[string]any{
					"type": "http",
					"url":  testURL,
					"headers": map[string]string{
						"Authorization": "${SAG_JWT}",
					},
				},
			},
		}
		data, _ := json.Marshal(entry)
		if err := os.MkdirAll(filepath.Dir(mcpFile), 0o700); err != nil {
			t.Fatalf("DXAC51 variable-header: mkdir: %v", err)
		}
		if err := os.WriteFile(mcpFile, data, 0o600); err != nil {
			t.Fatalf("DXAC51 variable-header: write mcp file: %v", err)
		}

		env := dxac51Env(xdgDir, claudeDir, binDir)
		cmd := exec.CommandContext(ctx, sagBin, "agent", "setup", "claude-code",
			"--url", testURL, "--sub", testSub, "--deny-rules", "no")
		cmd.Env = env
		cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		runErr := cmd.Run()
		code := 0
		if exitErr, ok := runErr.(*exec.ExitError); ok {
			code = exitErr.ExitCode()
		} else if runErr != nil {
			t.Fatalf("DXAC51 variable-header: run: %v", runErr)
		}
		if code != 1 {
			t.Fatalf("DXAC51 variable-header: exit %d; want 1\nstdout: %q\nstderr: %q",
				code, stdout.String(), stderr.String())
		}
		combined := stdout.String() + stderr.String()
		if !strings.Contains(combined, "another form") {
			t.Fatalf("DXAC51 variable-header: 'another form' missing\nstdout: %q\nstderr: %q",
				stdout.String(), stderr.String())
		}
	})
}

func dxac51SetupDirs(t *testing.T, fakeBin string) (xdgDir, binDir string) {
	t.Helper()
	xdgDir = t.TempDir()
	sagDir := filepath.Join(xdgDir, "sag")
	if err := os.MkdirAll(sagDir, 0o700); err != nil {
		t.Fatalf("DXAC51 setup: mkdir sag dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sagDir, "ca.pem"), []byte("fake-ca"), 0o600); err != nil {
		t.Fatalf("DXAC51 setup: write ca.pem: %v", err)
	}

	binDir = t.TempDir()
	if err := dxh.AddRealProgramLink(binDir, "claude", fakeBin); err != nil {
		t.Fatalf("DXAC51 setup: link claude: %v", err)
	}
	return xdgDir, binDir
}

func dxac51Env(xdgDir, claudeDir, binDir string) []string {
	env := []string{
		"XDG_CONFIG_HOME=" + xdgDir,
		"CLAUDE_CONFIG_DIR=" + claudeDir,
		"PI_CODING_AGENT_DIR=" + claudeDir,
		"PATH=" + dxh.TestPATH(binDir),
	}
	for _, e := range os.Environ() {
		key, _, _ := strings.Cut(e, "=")
		switch key {
		case "HOME", "TMPDIR", "USER", "LANG":
			env = append(env, e)
		}
	}
	return env
}

func dxac51FakePodman(t *testing.T, binDir string) string {
	t.Helper()
	recorder := filepath.Join(binDir, "podman-called")
	script := fmt.Sprintf("#!/bin/sh\ntouch %s\n", recorder)
	podmanPath := filepath.Join(binDir, "podman")
	if err := os.WriteFile(podmanPath, []byte(script), 0o755); err != nil {
		t.Fatalf("DXAC51: write fake podman: %v", err)
	}
	return recorder
}

func dxac51AssertMCPURL(t *testing.T, mcpFile, wantURL string) {
	t.Helper()
	data, err := os.ReadFile(mcpFile)
	if err != nil {
		t.Fatalf("DXAC51: read mcp file: %v", err)
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(data, &top); err != nil {
		t.Fatalf("DXAC51: parse mcp file: %v", err)
	}
	serversRaw, ok := top["mcpServers"]
	if !ok {
		t.Fatal("DXAC51: mcpServers missing")
	}
	var servers map[string]json.RawMessage
	if err := json.Unmarshal(serversRaw, &servers); err != nil {
		t.Fatalf("DXAC51: parse mcpServers: %v", err)
	}
	entryRaw, ok := servers["sag"]
	if !ok {
		t.Fatal("DXAC51: mcpServers.sag missing")
	}
	var entry struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(entryRaw, &entry); err != nil {
		t.Fatalf("DXAC51: parse sag entry: %v", err)
	}
	if entry.URL != wantURL {
		t.Fatalf("DXAC51: entry url = %q; want %q", entry.URL, wantURL)
	}
}

func dxac51Snapshot(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return ""
		}
		t.Fatalf("DXAC51 snapshot: %v", err)
	}
	return string(data)
}

func dxac51JWT(sub string) string {
	h := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	p := base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"` + sub + `"}`))
	s := base64.RawURLEncoding.EncodeToString([]byte("sig"))
	return h + "." + p + "." + s
}

func dxac51SnapshotDirs(t *testing.T, dirs ...string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, dir := range dirs {
		filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			data, err := os.ReadFile(path)
			if err == nil {
				out[path] = string(data)
			}
			return nil
		})
	}
	return out
}

func dxac51BuildFakeharness(t *testing.T, ctx context.Context) string {
	t.Helper()
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		t.Fatalf("DXAC51: git rev-parse: %v", err)
	}
	repoRoot := strings.TrimSpace(string(out))
	binPath := filepath.Join(t.TempDir(), "fakeharness")
	buildCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	build := exec.CommandContext(buildCtx, "go", "build", "-o", binPath, "./e2e/devx/cmd/fakeharness")
	build.Dir = repoRoot
	if buildOut, err := build.CombinedOutput(); err != nil {
		t.Fatalf("DXAC51: go build fakeharness: %v: %s", err, buildOut)
	}
	return binPath
}
