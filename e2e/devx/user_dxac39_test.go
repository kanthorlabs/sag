//go:build devxe2e

package devx_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/kanthorlabs/sag/e2e/devx/dxh"
	"github.com/kanthorlabs/sag/internal/agentsetup"
)

func init() {
	userHarnessTests["DXAC39_DefaultPath"] = func(t *testing.T, ctx context.Context) {
		DXAC39_DefaultPath(t, ctx)
	}
}

func DXAC39_DefaultPath(t *testing.T, ctx context.Context) {
	t.Helper()
	dxh.EnsureBootstrapped(t)

	sagBin, err := dxh.TestBinary()
	if err != nil {
		t.Fatalf("DXAC39-default: test binary: %v", err)
	}
	home := os.Getenv("HOME")
	cfgDir := dxh.CfgDir(home)
	cc := agentsetup.ClaudeCodePaths("", home)

	setupCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(setupCtx, sagBin, "agent", "setup", "claude-code", "--deny-rules", "no")
	cmd.Env = dxh.UserChildEnv(os.Environ(), userRealBinDir)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		t.Fatalf("DXAC39-default: agent setup claude-code: %v\n%s", err, dxh.FilterNoJWT(out.String()))
	}

	fi, err := os.Stat(cc.MCPFile)
	if err != nil {
		t.Fatalf("DXAC39-default: stat %s: %v", cc.MCPFile, err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Errorf("DXAC39-default: %s mode = %o; want 0600", cc.MCPFile, fi.Mode().Perm())
	}

	data, err := os.ReadFile(cc.MCPFile)
	if err != nil {
		t.Fatalf("DXAC39-default: read %s: %v", cc.MCPFile, err)
	}
	var top struct {
		MCPServers map[string]map[string]json.RawMessage `json:"mcpServers"`
	}
	if err := json.Unmarshal(data, &top); err != nil {
		t.Fatalf("DXAC39-default: parse %s: %v", cc.MCPFile, err)
	}
	entry, ok := top.MCPServers["sag"]
	if !ok {
		t.Fatalf("DXAC39-default: %s has no mcpServers.sag", cc.MCPFile)
	}
	if _, has := entry["command"]; has {
		t.Error("DXAC39-default: mcpServers.sag has a command key; want none")
	}
	var entryType, entryURL string
	if json.Unmarshal(entry["type"], &entryType) != nil || entryType != "http" {
		t.Errorf("DXAC39-default: mcpServers.sag type = %q; want http", entryType)
	}
	if json.Unmarshal(entry["url"], &entryURL) != nil || entryURL != "https://localhost:8443/mcp" {
		t.Errorf("DXAC39-default: mcpServers.sag url = %q; want https://localhost:8443/mcp", entryURL)
	}
	var headers map[string]string
	if err := json.Unmarshal(entry["headers"], &headers); err != nil {
		t.Fatalf("DXAC39-default: parse headers: %v", err)
	}
	jwtStr, hasBearer := strings.CutPrefix(headers["Authorization"], "Bearer ")
	if !hasBearer || jwtStr == "" {
		t.Fatal("DXAC39-default: Authorization header has no Bearer token")
	}

	skill, err := os.ReadFile(cc.SkillFile)
	if err != nil {
		t.Fatalf("DXAC39-default: read %s: %v", cc.SkillFile, err)
	}
	if !strings.Contains(string(skill), "<!-- managed by sag agent setup -->") {
		t.Errorf("DXAC39-default: %s has no managed marker", cc.SkillFile)
	}
	if !strings.Contains(string(skill), "--agent claude-code") {
		t.Errorf("DXAC39-default: %s has no --agent claude-code", cc.SkillFile)
	}

	if _, err := dxac39MCPClient(t, ctx, cfgDir, jwtStr); err != nil {
		t.Errorf("DXAC39-default: JWT does not authenticate at /mcp: %v", err)
	}
	var claims jwt.MapClaims
	if _, _, err := jwt.NewParser(jwt.WithoutClaimsValidation()).ParseUnverified(jwtStr, &claims); err != nil {
		t.Fatalf("DXAC39-default: parse JWT: %v", err)
	}
	if sub, _ := claims["sub"].(string); sub != "claude-code" {
		t.Errorf("DXAC39-default: JWT sub = %q; want claude-code", sub)
	}
}
