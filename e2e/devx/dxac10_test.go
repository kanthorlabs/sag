//go:build devxe2e

package devx_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
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

	canary := dxh.ActiveCanaryParts
	needles := canaryNeedles(canary)

	home := os.Getenv("HOME")
	cfgDir := dxh.CfgDir(home)
	uid := fmt.Sprintf("%d", os.Getuid())

	t.Log("collect sources")
	var sources []string

	listOut, listErr := runSAGWithStderr(t, ctx, "credential", "list")
	sources = append(sources, listOut, listErr)

	inspectOut, inspectErr := runSAGWithStderr(t, ctx, "credential", "inspect", "canary")
	sources = append(sources, inspectOut, inspectErr)

	r := dxh.NewRunner()

	podmanLogsOut, _ := r.Output(ctx, "podman", "logs", "sag")
	sources = append(sources, string(podmanLogsOut))

	podmanInspectOut, _ := r.Output(ctx, "podman", "container", "inspect", "sag")
	sources = append(sources, string(podmanInspectOut))

	secretNames := collectSecretNames(ctx, r)
	for _, sn := range secretNames {
		siOut, _ := r.Output(ctx, "podman", "secret", "inspect", sn)
		sources = append(sources, string(siOut))
	}

	auditOut, _ := r.Output(ctx, "podman", "exec", "sag", "sag", "audit", "read")
	sources = append(sources, string(auditOut))

	serviceLogPath := filepath.Join(home, "Library", "Logs", "sag", "service.log")
	if data, err := os.ReadFile(serviceLogPath); err == nil {
		sources = append(sources, string(data))
	}

	answersPath := filepath.Join(cfgDir, "answers.json")
	if data, err := os.ReadFile(answersPath); err == nil {
		sources = append(sources, string(data))
	}

	if data, err := readDirFiles(cfgDir); err == nil {
		sources = append(sources, data...)
	}

	launchAgentPath := filepath.Join(home, "Library", "LaunchAgents", "net.kanthorlabs.sag.plist")
	if data, err := os.ReadFile(launchAgentPath); err == nil {
		sources = append(sources, string(data))
	}

	t.Log("search for canary parts in sources")
	found := dxh.SearchStrings(sources, needles)
	if len(found) > 0 {
		t.Fatalf("canary parts found in sources: %v (content not logged per security rules)", found)
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

func readDirFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var contents []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		contents = append(contents, string(data))
	}
	return contents, nil
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
