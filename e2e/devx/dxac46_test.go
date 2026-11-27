//go:build devxe2e

package devx_test

import (
	"bytes"
	"context"
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
	bootagentTests["DXAC46_BootstrapAgentSetup"] = DXAC46_BootstrapAgentSetup
}

func DXAC46_BootstrapAgentSetup(t *testing.T, ctx context.Context) {
	t.Helper()

	sagBin, err := dxh.TestBinary()
	if err != nil {
		t.Fatalf("DXAC46: test binary: %v", err)
	}

	binDir := t.TempDir()
	if err := dxac46BuildFakeHarness(binDir); err != nil {
		t.Fatalf("DXAC46: build fakeharness: %v", err)
	}

	home := os.Getenv("HOME")
	cfgDir := dxh.CfgDir(home)
	uid := fmt.Sprintf("%d", os.Getuid())
	r := dxh.NewRunner()
	tcfg := dxh.TeardownConfig{Home: home, UID: uid, CfgDir: cfgDir}

	t.Run("happy", func(t *testing.T) {
		dxac46AssertPrecondition(t, ctx, r, cfgDir)

		groupDir := t.TempDir()
		env, err := dxh.ChildEnv(os.Environ(), groupDir)
		if err != nil {
			t.Fatalf("child env: %v", err)
		}
		env = append(env, "PATH="+dxh.TestPATH(binDir))
		claudeDir := filepath.Join(groupDir, "claude")
		piDir := filepath.Join(groupDir, "pi")
		piSnap := dxac46Snapshot(piDir)

		testCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
		defer cancel()

		cmd := exec.CommandContext(testCtx, sagBin, "bootstrap",
			"--autostart", "no",
			"--agent-setup", "claude-code=yes",
			"--agent-setup", "pi=no",
			"--deny-rules", "no")
		cmd.Env = env
		var stdoutBuf, stderrBuf strings.Builder
		cmd.Stdout = &stdoutBuf
		cmd.Stderr = &stderrBuf
		runErr := cmd.Run()
		code := 0
		if runErr != nil {
			if exitErr, ok := runErr.(*exec.ExitError); ok {
				code = exitErr.ExitCode()
			} else {
				t.Fatalf("run: %v", runErr)
			}
		}
		if code != 0 {
			t.Fatalf("exit code = %d; want 0; stdout=%s stderr=%s",
				code, stdoutBuf.String(), stderrBuf.String())
		}
		combined := stdoutBuf.String() + stderrBuf.String()
		if !strings.Contains(combined, "--agent claude-code") {
			t.Errorf("output missing --agent claude-code; got:\n%s", combined)
		}
		if strings.Contains(combined, "--agent pi") {
			t.Errorf("output has --agent pi; want absent; got:\n%s", combined)
		}
		dxac46AssertClaudeMCP(t, claudeDir)
		dxac46AssertPiUnchanged(t, piDir, piSnap)
	})

	if err := dxh.Teardown(ctx, r, tcfg); err != nil {
		t.Fatalf("DXAC46: teardown after happy: %v", err)
	}

	t.Run("noterm", func(t *testing.T) {
		env, err := dxh.ChildEnv(os.Environ(), t.TempDir())
		if err != nil {
			t.Fatalf("child env: %v", err)
		}
		env = append(env, "PATH="+dxh.TestPATH(binDir))

		testCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()

		cmd := exec.CommandContext(testCtx, sagBin, "bootstrap", "--autostart", "no")
		cmd.Env = env
		cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
		var stdoutBuf, stderrBuf strings.Builder
		cmd.Stdout = &stdoutBuf
		cmd.Stderr = &stderrBuf
		runErr := cmd.Run()
		code := 0
		if runErr != nil {
			if exitErr, ok := runErr.(*exec.ExitError); ok {
				code = exitErr.ExitCode()
			} else {
				t.Fatalf("run: %v", runErr)
			}
		}
		if code != 3 {
			t.Fatalf("exit code = %d; want 3; stdout=%s stderr=%s",
				code, stdoutBuf.String(), stderrBuf.String())
		}
		combined := stdoutBuf.String() + stderrBuf.String()
		if !strings.Contains(combined, "--agent-setup") {
			t.Errorf("output does not name --agent-setup; got:\n%s", combined)
		}
	})

	t.Run("catrust", func(t *testing.T) {
		dxac46AssertPrecondition(t, ctx, r, cfgDir)

		caFile := filepath.Join(t.TempDir(), "other-ca.pem")
		if err := os.WriteFile(caFile, []byte("not-a-cert"), 0o600); err != nil {
			t.Fatalf("write fake ca file: %v", err)
		}

		groupDir := t.TempDir()
		env, err := dxh.ChildEnv(os.Environ(), groupDir)
		if err != nil {
			t.Fatalf("child env: %v", err)
		}
		env = append(env, "PATH="+dxh.TestPATH(binDir))
		env = append(env, "NODE_EXTRA_CA_CERTS="+caFile)
		claudeDir := filepath.Join(groupDir, "claude")
		piDir := filepath.Join(groupDir, "pi")
		claudeSnap := dxac46Snapshot(claudeDir)
		piSnap := dxac46Snapshot(piDir)

		testCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
		defer cancel()

		cmd := exec.CommandContext(testCtx, sagBin, "bootstrap",
			"--autostart", "no",
			"--agent-setup", "claude-code=yes",
			"--agent-setup", "pi=no",
			"--deny-rules", "no")
		cmd.Env = env
		var stdoutBuf, stderrBuf strings.Builder
		cmd.Stdout = &stdoutBuf
		cmd.Stderr = &stderrBuf
		runErr := cmd.Run()
		code := 0
		if runErr != nil {
			if exitErr, ok := runErr.(*exec.ExitError); ok {
				code = exitErr.ExitCode()
			} else {
				t.Fatalf("run: %v", runErr)
			}
		}
		combined := stdoutBuf.String() + stderrBuf.String()
		if code != 1 {
			t.Errorf("exit code = %d; want 1; stdout=%s stderr=%s",
				code, stdoutBuf.String(), stderrBuf.String())
		}
		if !strings.Contains(combined, "NODE_EXTRA_CA_CERTS=") {
			t.Errorf("output missing bundle command line (NODE_EXTRA_CA_CERTS=...); got:\n%s", combined)
		}
		if err := healthzOK(ctx, filepath.Join(cfgDir, "ca.pem")); err != nil {
			t.Errorf("/healthz after ca-trust exit: %v", err)
		}
		dxac46AssertSnapshotUnchanged(t, "claude", claudeDir, claudeSnap)
		dxac46AssertSnapshotUnchanged(t, "pi", piDir, piSnap)
	})
}

func dxac46AssertPrecondition(t *testing.T, ctx context.Context, r dxh.Runner, cfgDir string) {
	t.Helper()
	if _, err := os.Stat(cfgDir); err == nil {
		t.Fatalf("precondition: cfg dir %s exists", cfgDir)
	}
	out, _ := r.Output(ctx, "podman", "ps", "-a", "--filter", "name=^sag$", "--format", "{{.Names}}")
	if strings.TrimSpace(string(out)) != "" {
		t.Fatalf("precondition: container sag exists")
	}
}

type dxac46FileSnap struct {
	mode os.FileMode
	data []byte
}

func dxac46Snapshot(dir string) map[string]dxac46FileSnap {
	snap := make(map[string]dxac46FileSnap)
	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		info, err2 := d.Info()
		if err2 != nil {
			return nil
		}
		data, err2 := os.ReadFile(path)
		if err2 != nil {
			return nil
		}
		rel, _ := filepath.Rel(dir, path)
		snap[rel] = dxac46FileSnap{mode: info.Mode(), data: data}
		return nil
	})
	return snap
}

func dxac46AssertSnapshotUnchanged(t *testing.T, name, dir string, before map[string]dxac46FileSnap) {
	t.Helper()
	after := dxac46Snapshot(dir)
	for rel, snap := range before {
		cur, ok := after[rel]
		if !ok {
			t.Errorf("%s: file %s removed after bootstrap", name, rel)
			continue
		}
		if cur.mode != snap.mode {
			t.Errorf("%s: file %s mode changed: %v → %v", name, rel, snap.mode, cur.mode)
		}
		if !bytes.Equal(cur.data, snap.data) {
			t.Errorf("%s: file %s content changed", name, rel)
		}
	}
	for rel := range after {
		if _, ok := before[rel]; !ok {
			t.Errorf("%s: file %s added after bootstrap", name, rel)
		}
	}
}

func dxac46AssertClaudeMCP(t *testing.T, claudeDir string) {
	t.Helper()
	mcpFile := filepath.Join(claudeDir, ".claude.json")
	data, err := os.ReadFile(mcpFile)
	if err != nil {
		t.Errorf("claude .claude.json: %v", err)
		return
	}
	var top struct {
		MCPServers map[string]json.RawMessage `json:"mcpServers"`
	}
	if err := json.Unmarshal(data, &top); err != nil {
		t.Errorf("claude .claude.json parse: %v", err)
		return
	}
	if _, ok := top.MCPServers["sag"]; !ok {
		var keys []string
		for k := range top.MCPServers {
			keys = append(keys, k)
		}
		t.Errorf("claude .claude.json missing mcpServers.sag; got keys: %v", keys)
	}
}

func dxac46AssertPiUnchanged(t *testing.T, piDir string, before map[string]dxac46FileSnap) {
	t.Helper()
	after := dxac46Snapshot(piDir)
	for rel := range after {
		if _, ok := before[rel]; !ok {
			t.Errorf("pi: unexpected file %s added; want no pi agent setup", rel)
		}
	}
	mcpFile := filepath.Join(piDir, "mcp.json")
	if data, err := os.ReadFile(mcpFile); err == nil {
		if strings.Contains(string(data), `"sag"`) {
			t.Errorf("pi mcp.json has sag entry; want absent (pi=no)")
		}
	}
	guardFile := filepath.Join(piDir, "extensions", "sag-guard.ts")
	if _, err := os.Stat(guardFile); err == nil {
		t.Errorf("pi extensions/sag-guard.ts exists; want absent (pi=no)")
	}
}

func dxac46BuildFakeHarness(binDir string) error {
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return fmt.Errorf("git rev-parse: %w", err)
	}
	root := strings.TrimSpace(string(out))
	fakeBin := filepath.Join(binDir, "fakeharness")
	cmd := exec.Command("go", "build", "-o", fakeBin, "./e2e/devx/cmd/fakeharness/")
	cmd.Dir = root
	cmd.Env = os.Environ()
	if data, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("go build fakeharness: %w\n%s", err, data)
	}
	for _, name := range []string{"claude", "pi"} {
		link := filepath.Join(binDir, name)
		_ = os.Remove(link)
		if err := os.Symlink(fakeBin, link); err != nil {
			return fmt.Errorf("symlink %s: %w", name, err)
		}
	}
	return nil
}
