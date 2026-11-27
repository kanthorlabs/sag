//go:build devxe2e

package devx_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kanthorlabs/sag/e2e/devx/dxh"
)

func init() {
	agentTests["DXAC40_JWTNoLeak"] = func(t *testing.T, ctx context.Context) {
		DXAC40_JWTNoLeak(t, ctx)
	}
}

func DXAC40_JWTNoLeak(t *testing.T, ctx context.Context) {
	t.Helper()
	dxh.EnsureAgentSetup(t)

	home := os.Getenv("HOME")
	cfgDir := dxh.CfgDir(home)

	jwts := dxac40CollectJWTs(t)
	if len(jwts) == 0 {
		t.Fatal("DXAC40: no JWTs found in MCP configuration files; EnsureAgentSetup must create them")
	}

	sources := dxac40CollectSources(t, ctx, home, cfgDir)

	mcpSources := map[string]bool{}
	if dxh.ActiveAgentClaudeDir != "" {
		mcpSources[filepath.Join(dxh.ActiveAgentClaudeDir, ".claude.json")] = true
	}
	if dxh.ActiveAgentPiDir != "" {
		mcpSources[filepath.Join(dxh.ActiveAgentPiDir, "mcp.json")] = true
	}

	var leaks []string
	for srcName, content := range sources {
		if _, isMCP := mcpSources[srcName]; isMCP {
			continue
		}
		for _, jwt := range jwts {
			if strings.Contains(content, jwt) {
				leaks = append(leaks, "JWT found in "+srcName)
			}
		}
	}
	if len(leaks) > 0 {
		t.Fatalf("DXAC40: JWT leak detected:\n%s", strings.Join(leaks, "\n"))
	}

	for srcName, content := range sources {
		if _, isMCP := mcpSources[srcName]; isMCP {
			for _, jwt := range jwts {
				if !strings.Contains(content, jwt) {
					t.Logf("DXAC40: positive control: JWT not found in MCP source %s (not an error if keys changed)", srcName)
				}
			}
		}
	}
}

func dxac40CollectJWTs(t *testing.T) []string {
	t.Helper()
	var jwts []string
	if dxh.ActiveAgentClaudeDir != "" {
		if jwt := dxac40ExtractJWT(t, filepath.Join(dxh.ActiveAgentClaudeDir, ".claude.json"), "mcpServers", "sag"); jwt != "" {
			jwts = append(jwts, jwt)
		}
	}
	if dxh.ActiveAgentPiDir != "" {
		if jwt := dxac40ExtractJWT(t, filepath.Join(dxh.ActiveAgentPiDir, "mcp.json"), "mcpServers", "sag"); jwt != "" {
			jwts = append(jwts, jwt)
		}
	}
	return jwts
}

func dxac40ExtractJWT(t *testing.T, mcpFile, topKey, serverKey string) string {
	t.Helper()
	data, err := os.ReadFile(mcpFile)
	if err != nil {
		return ""
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(data, &top); err != nil {
		return ""
	}
	raw, ok := top[topKey]
	if !ok {
		return ""
	}
	var servers map[string]json.RawMessage
	if err := json.Unmarshal(raw, &servers); err != nil {
		return ""
	}
	entry, ok := servers[serverKey]
	if !ok {
		return ""
	}
	var sag struct {
		Headers struct {
			Authorization string `json:"Authorization"`
		} `json:"headers"`
	}
	if err := json.Unmarshal(entry, &sag); err != nil {
		return ""
	}
	auth := sag.Headers.Authorization
	if !strings.HasPrefix(auth, "Bearer ") {
		return ""
	}
	return strings.TrimPrefix(auth, "Bearer ")
}

func dxac40CollectSources(t *testing.T, ctx context.Context, home, cfgDir string) map[string]string {
	t.Helper()
	sources := map[string]string{}

	for _, out := range dxh.ActiveAgentSetupOutput {
		sources["agent setup output"] += out
	}

	r := dxh.NewRunner()

	podmanLogsOut, err := r.Output(ctx, "podman", "logs", "--timestamps=false", "sag")
	if err == nil {
		sources["podman logs sag"] = string(podmanLogsOut)
	}

	auditCopy := t.TempDir()
	if out, err := r.Output(ctx, "podman", "cp", "sag:/var/lib/sag/audit", auditCopy); err == nil {
		_ = out
		auditFiles, err := dxac40ReadDirFiles(filepath.Join(auditCopy, "audit"))
		if err == nil {
			for name, content := range auditFiles {
				sources["audit "+filepath.Base(name)] = content
			}
		}
	}

	serviceLogPath := filepath.Join(home, "Library", "Logs", "sag", "service.log")
	if data, err := os.ReadFile(serviceLogPath); err == nil {
		sources["service log"] = string(data)
	}

	cfgFiles, err := dxac40ReadDirFiles(cfgDir)
	if err == nil {
		for name, content := range cfgFiles {
			sources["cfg:"+name] = content
		}
	}

	if out, err := r.Output(ctx, "podman", "container", "inspect", "sag"); err == nil {
		sources["podman container inspect sag"] = string(out)
	}

	secretNames := dxac40CollectSecretNames(ctx, r)
	for _, sn := range secretNames {
		if out, err := r.Output(ctx, "podman", "secret", "inspect", sn); err == nil {
			sources["podman secret inspect "+sn] = string(out)
		}
	}

	if dxh.ActiveAgentBinDir != "" {
		fakeRecordDir := filepath.Join(dxh.ActiveAgentBinDir, "records")
		if records, err := dxac40ReadDirFiles(fakeRecordDir); err == nil {
			for name, content := range records {
				sources["fakeharness record "+filepath.Base(name)] = content
			}
		}
	}

	return sources
}

func dxac40ReadDirFiles(dir string) (map[string]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, e := range entries {
		if e.IsDir() {
			sub, err := dxac40ReadDirFiles(filepath.Join(dir, e.Name()))
			if err == nil {
				for k, v := range sub {
					out[k] = v
				}
			}
			continue
		}
		p := filepath.Join(dir, e.Name())
		data, err := os.ReadFile(p)
		if err == nil {
			out[p] = string(data)
		}
	}
	return out, nil
}

func dxac40CollectSecretNames(ctx context.Context, r dxh.Runner) []string {
	out, err := r.Output(ctx, "podman", "secret", "ls", "--format", "{{.Name}}")
	if err != nil {
		return nil
	}
	var names []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			names = append(names, line)
		}
	}
	return names
}

func dxac40PodmanExec(ctx context.Context, r dxh.Runner, args ...string) string {
	out, _ := r.Output(ctx, "podman", append([]string{"exec", "sag"}, args...)...)
	return string(out)
}
