//go:build devxe2e

package devx_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
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
	if len(jwts) < 2 {
		t.Fatalf("DXAC40: found %d JWT(s); EnsureAgentSetup must create one per harness (claude-code and pi)", len(jwts))
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
					t.Errorf("DXAC40: positive control: JWT not found in MCP source %s", srcName)
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
	bin, err := dxh.TestBinary()
	if err != nil {
		t.Fatalf("DXAC40: test binary: %v", err)
	}
	sources := map[string]string{}

	agentOut := ""
	for _, out := range dxh.ActiveAgentSetupOutput {
		agentOut += out
	}
	if agentOut == "" {
		t.Error("DXAC40: agent setup output is empty; EnsureAgentSetup must have run")
	}
	sources["agent setup output"] = agentOut

	if dxh.ActiveAgentBinDir != "" {
		recordPath := filepath.Join(dxh.ActiveAgentBinDir, "stub_records.json")
		data, err := os.ReadFile(recordPath)
		if err != nil {
			t.Fatalf("DXAC40: stub_records.json not found at %s: %v", recordPath, err)
		}
		if len(data) == 0 {
			t.Fatal("DXAC40: stub_records.json is empty; fakeharness must have been called")
		}
		sources["stub_records"] = string(data)
	}

	r := dxh.NewRunner()

	podmanLogsOut, err := exec.CommandContext(ctx, "podman", "logs", "--timestamps=false", "sag").CombinedOutput()
	if err != nil {
		t.Fatalf("DXAC40: podman logs sag: %v\n%s", err, podmanLogsOut)
	}
	if len(podmanLogsOut) == 0 {
		t.Fatal("DXAC40: podman logs sag: empty")
	}
	sources["podman logs sag"] = string(podmanLogsOut)

	auditOut, err := r.Output(ctx, "podman", "exec", "sag", bin, "audit", "read")
	if err != nil {
		t.Fatalf("DXAC40: audit read: %v", err)
	}
	if len(auditOut) == 0 {
		t.Fatal("DXAC40: audit read: empty; EnsureAgentSetup must have produced audit entries")
	}
	sources["audit read"] = string(auditOut)

	serviceLogPath := filepath.Join(home, "Library", "Logs", "sag", "service.log")
	serviceData, err := os.ReadFile(serviceLogPath)
	if err != nil {
		t.Fatalf("DXAC40: service log %s: %v", serviceLogPath, err)
	}
	if len(serviceData) == 0 {
		t.Fatalf("DXAC40: service log %s: empty", serviceLogPath)
	}
	sources["service log"] = string(serviceData)

	cfgFiles, err := dxac40ReadDirFiles(cfgDir)
	if err != nil {
		t.Fatalf("DXAC40: read cfg dir %s: %v", cfgDir, err)
	}
	if len(cfgFiles) == 0 {
		t.Fatalf("DXAC40: cfg dir %s: no files found", cfgDir)
	}
	for name, content := range cfgFiles {
		sources["cfg:"+name] = content
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
