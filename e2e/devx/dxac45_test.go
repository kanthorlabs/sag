//go:build devxe2e

package devx_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/kanthorlabs/sag/e2e/devx/dxh"
)

func init() {
	agentTests["DXAC45_AgentIssue"] = func(t *testing.T, ctx context.Context) {
		DXAC45_AgentIssue(t, ctx)
	}
}

func DXAC45_AgentIssue(t *testing.T, ctx context.Context) {
	t.Helper()
	dxh.EnsureBootstrapped(t)

	sagBin, err := dxh.TestBinary()
	if err != nil {
		t.Fatalf("DXAC45: test binary: %v", err)
	}

	groupDir := t.TempDir()
	binDir := t.TempDir()
	harnessDir := dxac39BuildFakeHarness(t, binDir)
	_ = harnessDir

	env, err := dxh.ChildEnv(os.Environ(), groupDir)
	if err != nil {
		t.Fatalf("DXAC45: child env: %v", err)
	}
	claudeDir := dxac39FindVal(env, "CLAUDE_CONFIG_DIR")
	if claudeDir == "" {
		t.Fatal("DXAC45: CLAUDE_CONFIG_DIR not set")
	}
	env = append(env, "PATH="+dxh.TestPATH(binDir))
	home := os.Getenv("HOME")
	if home != "" {
		env = append(env, "HOME="+home)
	}

	sagCfgDir := dxh.CfgDir(home)

	if err := os.MkdirAll(claudeDir, 0o700); err != nil {
		t.Fatalf("DXAC45: mkdir claude dir: %v", err)
	}

	runAgentSetup := func(t *testing.T, ctx context.Context, extraArgs ...string) (int, string) {
		t.Helper()
		args := append([]string{"agent", "setup", "claude-code"}, extraArgs...)
		cmd := exec.CommandContext(ctx, sagBin, args...)
		cmd.Env = env
		cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
		var outBuf bytes.Buffer
		cmd.Stdout = &outBuf
		cmd.Stderr = &outBuf
		runErr := cmd.Run()
		code := 0
		if runErr != nil {
			if exitErr, ok := runErr.(*exec.ExitError); ok {
				code = exitErr.ExitCode()
			} else {
				t.Fatalf("DXAC45: sag %v: %v", args, runErr)
			}
		}
		return code, outBuf.String()
	}

	readMCPEntry := func(t *testing.T) []byte {
		t.Helper()
		mcpFile := filepath.Join(claudeDir, ".claude.json")
		data, err := os.ReadFile(mcpFile)
		if err != nil {
			t.Fatalf("DXAC45: read .claude.json: %v", err)
		}
		return data
	}

	jwtFromMCP := func(t *testing.T) string {
		t.Helper()
		data := readMCPEntry(t)
		var top map[string]json.RawMessage
		if err := json.Unmarshal(data, &top); err != nil {
			t.Fatalf("DXAC45: unmarshal .claude.json: %v", err)
		}
		var servers map[string]json.RawMessage
		if err := json.Unmarshal(top["mcpServers"], &servers); err != nil {
			t.Fatalf("DXAC45: unmarshal mcpServers: %v", err)
		}
		sagRaw := servers["sag"]
		var sagEntry map[string]json.RawMessage
		if err := json.Unmarshal(sagRaw, &sagEntry); err != nil {
			t.Fatalf("DXAC45: unmarshal sag entry: %v", err)
		}
		var headers map[string]string
		if err := json.Unmarshal(sagEntry["headers"], &headers); err != nil {
			t.Fatalf("DXAC45: unmarshal headers: %v", err)
		}
		auth := headers["Authorization"]
		if !strings.HasPrefix(auth, "Bearer ") {
			t.Fatalf("DXAC45: Authorization header missing Bearer prefix")
		}
		return strings.TrimPrefix(auth, "Bearer ")
	}

	t.Run("setup_without_resource_exits_0", func(t *testing.T) {
		code, out := runAgentSetup(t, ctx)
		if code != 0 {
			t.Errorf("DXAC45-1: exit = %d; want 0; output length = %d", code, len(out))
		}
	})

	jwtStr := jwtFromMCP(t)

	t.Run("list_resources_empty_before_credential", func(t *testing.T) {
		names, err := dxac39MCPClient(t, ctx, sagCfgDir, jwtStr)
		if err != nil {
			t.Fatalf("DXAC45-2: list_resources: %v", err)
		}
		if len(names) != 0 {
			t.Errorf("DXAC45-2: list_resources = %v; want empty", names)
		}
	})

	t.Run("jwt_generate_unlisted_sub_exit1_before_credential", func(t *testing.T) {
		tty, err := dxh.StartTTY(ctx, t.TempDir(), t.TempDir(), sagBin, "jwt", "generate", "claude-code")
		if err != nil {
			t.Fatalf("DXAC45-3: start tty: %v", err)
		}
		if err := tty.Expect(ctx, "exit"); err != nil {
			if err2 := tty.Expect(ctx, "is not in"); err2 != nil {
				t.Fatalf("DXAC45-3: neither 'exit' nor error message found: %v", err2)
			}
		}
		code, waitErr := tty.Wait(ctx)
		if waitErr != nil && code == 0 {
			t.Fatalf("DXAC45-3: wait: %v", waitErr)
		}
		raw := tty.RawOutput()
		if strings.Contains(raw, "eyJ") {
			t.Fatal("DXAC45-3: PTY output contains a JWT; want none for unlisted sub")
		}
		if code == 0 {
			t.Errorf("DXAC45-3: exit = 0; want non-zero (sub not in any resource agents list)")
		}
	})

	credName := fmt.Sprintf("dxac45-%d", time.Now().UnixMilli()%100000)
	t.Cleanup(func() {
		ctx2, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		dxac45DeleteCred(t, ctx2, sagBin, credName)
	})

	t.Run("credential_create_with_agent", func(t *testing.T) {
		dsn := "postgres://u:p@127.0.0.1:1/dxac45db"
		code, out := runSAGWithInput(t, ctx, []byte(dsn),
			"credential", "create", credName,
			"--kind", "postgres",
			"--agent", "claude-code",
			"--tls", "disable",
			"--stdin",
			"--accept-failed-check",
		)
		if code != 0 {
			t.Fatalf("DXAC45-cred: credential create exit %d; output length=%d", code, len(out))
		}
	})

	uid := fmt.Sprintf("%d", os.Getuid())
	if err := dxh.WaitLaunchAgentIdle(ctx, dxh.NewRunner(), uid); err != nil {
		t.Fatalf("DXAC45: wait idle after credential create: %v", err)
	}

	t.Run("list_resources_returns_credential", func(t *testing.T) {
		names, err := dxac39MCPClient(t, ctx, sagCfgDir, jwtStr)
		if err != nil {
			t.Fatalf("DXAC45-4: list_resources: %v", err)
		}
		found := false
		for _, n := range names {
			if n == credName {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("DXAC45-4: list_resources does not contain %q; got %v", credName, names)
		}
	})

	t.Run("sub_bad_exit2", func(t *testing.T) {
		code, _ := runAgentSetup(t, ctx, "--sub", "Bad")
		if code != 2 {
			t.Errorf("DXAC45-5: exit = %d; want 2 (ExitUsage) for invalid sub 'Bad'", code)
		}
	})

	t.Run("podman_exec_t_exit3_no_jwt", func(t *testing.T) {
		r := dxh.NewRunner()
		testCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		out, err := r.Output(testCtx, "podman", "exec", "-t", "sag", "sag", "agent", "issue", "claude-code")
		combined := string(out)
		code := 0
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				code = exitErr.ExitCode()
			}
		}
		if strings.Contains(combined, "eyJ") {
			t.Error("DXAC45-6: podman exec -t output contains a JWT; want none")
		}
		if code != 3 {
			t.Errorf("DXAC45-6: podman exec -t sag agent issue exit = %d; want 3 (ExitRefusal)", code)
		}
	})

	t.Run("second_setup_keeps_entry_byte_equal", func(t *testing.T) {
		entryBefore := readMCPEntry(t)
		code, out := runAgentSetup(t, ctx)
		if code != 0 {
			t.Fatalf("DXAC45-7: second setup exit %d; output length=%d", code, len(out))
		}
		entryAfter := readMCPEntry(t)
		if !bytes.Equal(entryBefore, entryAfter) {
			t.Error("DXAC45-7: .claude.json changed on second setup; want byte-equal")
		}
	})

	t.Run("sub_other_writes_new_jwt", func(t *testing.T) {
		code, out := runAgentSetup(t, ctx, "--sub", "other")
		if code != 0 {
			t.Fatalf("DXAC45-8: setup --sub other exit %d; output length=%d", code, len(out))
		}
		newJWT := jwtFromMCP(t)
		sub := dxac45JWTSub(t, newJWT)
		if sub != "other" {
			t.Errorf("DXAC45-8: JWT sub = %q; want other", sub)
		}
	})

	t.Run("auth_version_bump_new_jwt_authenticates", func(t *testing.T) {
		entryBefore := readMCPEntry(t)

		if err := dxac45BumpAuthVersion(t, sagCfgDir); err != nil {
			t.Fatalf("DXAC45-9: bump auth.version: %v", err)
		}

		if _, err := runSAG(t, ctx, "service", "stop"); err != nil {
			t.Fatalf("DXAC45-9: service stop: %v", err)
		}
		if _, err := runSAG(t, ctx, "service", "start"); err != nil {
			t.Fatalf("DXAC45-9: service start: %v", err)
		}
		if err := dxh.WaitLaunchAgentIdle(ctx, dxh.NewRunner(), uid); err != nil {
			t.Fatalf("DXAC45-9: wait idle after restart: %v", err)
		}
		if err := healthzOK(ctx, filepath.Join(sagCfgDir, "ca.pem")); err != nil {
			t.Fatalf("DXAC45-9: healthz after restart: %v", err)
		}

		code, out := runAgentSetup(t, ctx, "--sub", "claude-code")
		if code != 0 {
			t.Fatalf("DXAC45-9: setup after version bump exit %d; output length=%d", code, len(out))
		}

		entryAfter := readMCPEntry(t)
		if bytes.Equal(entryBefore, entryAfter) {
			t.Error("DXAC45-9: .claude.json unchanged after auth.version bump; want new JWT")
		}

		newJWT := jwtFromMCP(t)
		names, err := dxac39MCPClient(t, ctx, sagCfgDir, newJWT)
		if err != nil {
			t.Fatalf("DXAC45-9: list_resources with new JWT: %v", err)
		}
		found := false
		for _, n := range names {
			if n == credName {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("DXAC45-9: new JWT does not authenticate; list_resources missing %q; got %v",
				credName, names)
		}
	})
}

var jwtRe45 = regexp.MustCompile(`eyJ[A-Za-z0-9+/=_-]+\.[A-Za-z0-9+/=_-]+\.[A-Za-z0-9+/=_-]+`)

func dxac45JWTSub(t *testing.T, raw string) string {
	t.Helper()
	parts := strings.SplitN(raw, ".", 3)
	if len(parts) != 3 {
		t.Fatal("DXAC45: not a JWT")
	}
	var claims map[string]any
	payload, err := jwtBase64Decode(parts[1])
	if err != nil {
		t.Fatalf("DXAC45: decode JWT payload: %v", err)
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		t.Fatalf("DXAC45: unmarshal JWT claims: %v", err)
	}
	sub, _ := claims["sub"].(string)
	return sub
}

func jwtBase64Decode(s string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(s)
}

func dxac45BumpAuthVersion(t *testing.T, sagCfgDir string) error {
	t.Helper()
	yamlPath := filepath.Join(sagCfgDir, "sag.yaml")
	data, err := os.ReadFile(yamlPath)
	if err != nil {
		return fmt.Errorf("read sag.yaml: %w", err)
	}
	lines := strings.Split(string(data), "\n")
	sc := bufio.NewScanner(bytes.NewReader(data))
	var newLines []string
	inAuth := false
	bumpedVersion := false
	sc2 := bufio.NewScanner(bytes.NewReader(data))
	for sc2.Scan() {
		line := sc2.Text()
		if strings.HasPrefix(line, "auth:") {
			inAuth = true
			newLines = append(newLines, line)
			continue
		}
		if inAuth && strings.TrimSpace(strings.SplitN(line, ":", 2)[0]) == "version" {
			parts := strings.SplitN(line, ":", 2)
			n, err := strconv.Atoi(strings.TrimSpace(parts[1]))
			if err != nil {
				inAuth = false
			} else {
				newLines = append(newLines, parts[0]+": "+strconv.Itoa(n+1))
				bumpedVersion = true
				inAuth = false
				continue
			}
		} else if inAuth && !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") && line != "" {
			inAuth = false
		}
		newLines = append(newLines, line)
	}
	_ = lines
	_ = sc
	if !bumpedVersion {
		return fmt.Errorf("auth.version not found in sag.yaml; content length=%d", len(data))
	}
	newData := strings.Join(newLines, "\n")
	return os.WriteFile(yamlPath, []byte(newData), 0o644)
}

func dxac45DeleteCred(t *testing.T, ctx context.Context, sagBin, name string) {
	t.Helper()
	tty, err := dxh.StartTTY(ctx, t.TempDir(), t.TempDir(), sagBin, "credential", "delete", name)
	if err != nil {
		t.Logf("DXAC45 cleanup: start TTY for credential delete %s: %v", name, err)
		return
	}
	_ = tty.Expect(ctx, "confirm")
	_ = tty.Expect(ctx, "y/n")
	_ = tty.Expect(ctx, name)
}
