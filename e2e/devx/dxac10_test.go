//go:build devxe2e

package devx_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kanthorlabs/sag/e2e/devx/dxh"
)

var credentialGroupStart = time.Now()

func init() {
	credentialTests["DXAC10_CanaryNoLeak"] = func(t *testing.T, ctx context.Context) {
		DXAC10_CanaryNoLeak(t, ctx)
	}
}

func DXAC10_CanaryNoLeak(t *testing.T, ctx context.Context) {
	t.Helper()
	dxh.EnsureCanary(t)

	if len(dxh.ActiveCanaryTranscript) == 0 {
		t.Fatal("DXAC10: ActiveCanaryTranscript is empty; D4.5b must set it before D4.5c runs")
	}

	canary := dxh.ActiveCanaryParts
	if canary.Password == "" || canary.Host == "" || canary.User == "" || canary.Database == "" {
		t.Fatalf("DXAC10: canary parts incomplete: password=%v host=%v user=%v database=%v",
			canary.Password != "", canary.Host != "", canary.User != "", canary.Database != "")
	}

	home := os.Getenv("HOME")
	cfgDir := dxh.CfgDir(home)
	uid := fmt.Sprintf("%d", os.Getuid())

	if err := waitLaunchAgentIdle(ctx, uid); err != nil {
		t.Fatalf("waitLaunchAgentIdle: %v", err)
	}

	t.Log("collect sources")
	namedSources := map[string]string{}

	listOut, listErr, listRunErr := runSAGWithStderr(t, ctx, "credential", "list")
	if listRunErr != nil {
		t.Fatalf("credential list: %v", listRunErr)
	}
	namedSources["credential list stdout"] = listOut
	namedSources["credential list stderr"] = listErr

	inspectOut, inspectErr, inspectRunErr := runSAGWithStderr(t, ctx, "credential", "inspect", "canary")
	if inspectRunErr != nil {
		t.Fatalf("credential inspect canary: %v", inspectRunErr)
	}
	namedSources["credential inspect stdout"] = inspectOut
	namedSources["credential inspect stderr"] = inspectErr

	namedSources["canary create transcript"] = string(dxh.ActiveCanaryTranscript)

	podmanLogsCmd := exec.CommandContext(ctx, "podman", "logs", "--timestamps=false", "sag")
	podmanLogsOut, err := podmanLogsCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("podman logs sag: %v\n%s", err, podmanLogsOut)
	}
	if len(podmanLogsOut) == 0 {
		t.Fatal("DXAC10: podman logs sag: combined output is empty")
	}
	namedSources["podman logs sag"] = string(podmanLogsOut)

	r := dxh.NewRunner()

	podmanInspectOut, err := r.Output(ctx, "podman", "container", "inspect", "sag")
	if err != nil {
		t.Fatalf("podman container inspect sag: %v", err)
	}
	if len(podmanInspectOut) == 0 {
		t.Fatal("DXAC10: podman container inspect sag: output is empty")
	}
	namedSources["podman container inspect sag"] = string(podmanInspectOut)

	secretNames := collectSecretNames(ctx, r)
	if len(secretNames) == 0 {
		t.Fatal("DXAC10: no secrets found")
	}
	for _, sn := range secretNames {
		siOut, err := r.Output(ctx, "podman", "secret", "inspect", sn)
		if err != nil {
			t.Fatalf("podman secret inspect %s: %v", sn, err)
		}
		if len(siOut) == 0 {
			t.Fatalf("DXAC10: podman secret inspect %s: output is empty", sn)
		}
		namedSources["podman secret inspect "+sn] = string(siOut)
	}

	auditOut, err := r.Output(ctx, "podman", "exec", "sag", "sag", "audit", "read")
	if err != nil {
		t.Fatalf("audit read: %v", err)
	}
	namedSources["audit log"] = string(auditOut)

	serviceLogPath := filepath.Join(home, "Library", "Logs", "sag", "service.log")
	if data, err := os.ReadFile(serviceLogPath); err == nil {
		namedSources["service log"] = string(data)
	} else if !os.IsNotExist(err) {
		t.Fatalf("read service log: %v", err)
	}

	answersPath := filepath.Join(cfgDir, "answers.json")
	if data, err := os.ReadFile(answersPath); err == nil {
		namedSources["answers.json"] = string(data)
	} else if !os.IsNotExist(err) {
		t.Fatalf("read answers.json: %v", err)
	}

	cfgFiles, err := readNamedDirFiles(cfgDir)
	if err != nil {
		t.Fatalf("read cfg dir: %v", err)
	}
	for name, content := range cfgFiles {
		namedSources["cfg:"+name] = content
	}

	launchAgentPath := filepath.Join(home, "Library", "LaunchAgents", "net.kanthorlabs.sag.plist")
	if data, err := os.ReadFile(launchAgentPath); err == nil {
		namedSources["launch agent"] = string(data)
	} else if !os.IsNotExist(err) {
		t.Fatalf("read launch agent: %v", err)
	}

	t.Log("search for canary parts in sources")
	namedParts := map[string]string{
		"password": canary.Password,
		"host":     canary.Host,
		"user":     canary.User,
		"database": canary.Database,
	}
	findings := dxh.SearchNamedParts(namedSources, namedParts)
	if len(findings) > 0 {
		t.Fatalf("canary parts found in sources: %v", findings)
	}

	t.Log("scan for private key matching ca.pem")
	caPath := filepath.Join(cfgDir, "ca.pem")
	pubKey, err := readCAPubKey(caPath)
	if err != nil {
		t.Fatalf("read ca.pem public key: %v", err)
	}

	tmpDir := os.Getenv("TMPDIR")
	if tmpDir == "" {
		tmpDir = "/tmp"
	}
	if resolved, err := filepath.EvalSymlinks(tmpDir); err == nil {
		tmpDir = resolved
	}

	result, err := dxh.ScanForMatchingKey(ctx, home, tmpDir, pubKey, credentialGroupStart)
	if err != nil {
		t.Fatalf("key scan: %v", err)
	}
	if result.TimedOut {
		t.Fatal("key scan: timed out after 5 minutes")
	}
	if result.MatchFound {
		t.Fatal("key scan: found private key matching ca.pem public key (path not logged per security rules)")
	}
	t.Logf("key scan complete: denied dirs=%d, match_found=%v", result.DeniedDirs, result.MatchFound)
	_ = uid
}

func canaryNeedles(c dxh.CanaryParts) []string {
	var out []string
	if c.Password != "" {
		out = append(out, c.Password)
	}
	if c.Host != "" {
		out = append(out, c.Host)
	}
	if c.User != "" {
		out = append(out, c.User)
	}
	if c.Database != "" {
		out = append(out, c.Database)
	}
	return out
}

func collectSecretNames(ctx context.Context, r dxh.Runner) []string {
	out, err := r.Output(ctx, "podman", "secret", "ls", "--format", "{{.Name}}")
	if err != nil {
		return nil
	}
	var names []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		name := strings.TrimSpace(line)
		if name != "" {
			names = append(names, name)
		}
	}
	return names
}

func readNamedDirFiles(dir string) (map[string]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	result := make(map[string]string)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		result[e.Name()] = string(data)
	}
	return result, nil
}

func readDirFiles(dir string) ([]string, error) {
	named, err := readNamedDirFiles(dir)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, v := range named {
		out = append(out, v)
	}
	return out, nil
}

func readCAPubKey(caPath string) (*ecdsa.PublicKey, error) {
	data, err := os.ReadFile(caPath)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("ca.pem: no PEM block")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("ca.pem: parse certificate: %w", err)
	}
	pub, ok := cert.PublicKey.(*ecdsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("ca.pem: not an ECDSA public key")
	}
	return pub, nil
}
