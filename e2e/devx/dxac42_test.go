//go:build devxe2e

package devx_test

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"io/fs"
	"math/big"
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
	agentTests["DXAC42_CATrustConflict"] = func(t *testing.T, ctx context.Context) {
		DXAC42_CATrustConflict(t, ctx)
	}
}

func DXAC42_CATrustConflict(t *testing.T, ctx context.Context) {
	t.Helper()
	dxh.EnsureBootstrapped(t)

	bin, err := dxh.TestBinary()
	if err != nil {
		t.Fatalf("DXAC42: test binary: %v", err)
	}
	podmanPath, err := exec.LookPath("podman")
	if err != nil {
		t.Fatalf("DXAC42: find podman: %v", err)
	}

	fakeBinDir := dxh.ActiveAgentBinDir
	if fakeBinDir == "" {
		t.Fatal("DXAC42: dxh.ActiveAgentBinDir is not set")
	}

	t.Run("pi_env_conflict", func(t *testing.T) {
		dxac42PiEnvConflict(t, ctx, bin, podmanPath, fakeBinDir)
	})

	t.Run("claude_settings_conflict", func(t *testing.T) {
		dxac42ClaudeSettingsConflict(t, ctx, bin, podmanPath, fakeBinDir)
	})
}

func dxac42PiEnvConflict(t *testing.T, ctx context.Context, bin, podmanPath, fakeBinDir string) {
	t.Helper()
	piDir := t.TempDir()
	binDir := t.TempDir()
	cfgDir := dxh.CfgDir(os.Getenv("HOME"))
	bundlePath := dxac42PrepareBundlePath(t, cfgDir)
	otherCA := dxac42WriteOtherCA(t)
	if err := dxh.AddRealProgramLink(binDir, "podman", podmanPath); err != nil {
		t.Fatal(err)
	}
	if err := dxh.AddRealProgramLink(binDir, "pi", filepath.Join(fakeBinDir, "pi")); err != nil {
		t.Fatal(err)
	}
	if err := dxh.AddRealProgramLink(binDir, "claude", filepath.Join(fakeBinDir, "claude")); err != nil {
		t.Fatal(err)
	}

	env := dxac42Env(t, t.TempDir(), piDir, binDir)
	env = append(env, "NODE_EXTRA_CA_CERTS="+otherCA)
	env = append(env, "SHELL=/bin/zsh")
	zdotdir := t.TempDir()
	env = append(env, "ZDOTDIR="+zdotdir)

	snapBefore := dxac42Snapshot(piDir, zdotdir)

	code, out := dxac42RunSetsid(ctx, bin, env, "agent", "setup", "pi",
		"--shell-profile", "yes", "--guard", "yes")
	if code != 1 {
		t.Fatalf("DXAC42 pi-env: expected exit 1; got %d\n%s", code, out)
	}
	dxac42AssertConflictLine(t, "pi-env", out, otherCA, cfgDir, "the environment")

	snapAfter := dxac42Snapshot(piDir, zdotdir)
	dxac42AssertUnchanged(t, "pi-env conflict", snapBefore, snapAfter)

	bundleCmd := dxac42ParseBundleCmd(out)
	if bundleCmd == "" {
		t.Fatalf("DXAC42 pi-env: output holds no bundle command; got %q", out)
	}
	if err := dxac42RunBundleCmd(bundleCmd); err != nil {
		t.Fatalf("DXAC42 pi-env: run bundle command: %v", err)
	}
	if _, err := os.Stat(bundlePath); err != nil {
		t.Fatalf("DXAC42 pi-env: bundle file not created at %s: %v", bundlePath, err)
	}

	env2 := dxac42Env(t, t.TempDir(), piDir, binDir)
	env2 = append(env2, "NODE_EXTRA_CA_CERTS="+bundlePath)
	env2 = append(env2, "SHELL=/bin/zsh")
	env2 = append(env2, "ZDOTDIR="+zdotdir)
	code2, out2 := dxac42RunSetsid(ctx, bin, env2, "agent", "setup", "pi",
		"--shell-profile", "yes", "--guard", "yes")
	if code2 != 0 {
		t.Fatalf("DXAC42 pi-env: after bundle, expected exit 0; got %d\n%s", code2, out2)
	}
}

func dxac42ClaudeSettingsConflict(t *testing.T, ctx context.Context, bin, podmanPath, fakeBinDir string) {
	t.Helper()
	claudeDir := t.TempDir()
	binDir := t.TempDir()
	cfgDir := dxh.CfgDir(os.Getenv("HOME"))
	bundlePath := dxac42PrepareBundlePath(t, cfgDir)
	otherCA := dxac42WriteOtherCA(t)
	settings := filepath.Join(claudeDir, "settings.json")
	settingsContent := `{"env":{"NODE_EXTRA_CA_CERTS":"` + otherCA + `"}}`
	if err := os.WriteFile(settings, []byte(settingsContent), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := dxh.AddRealProgramLink(binDir, "podman", podmanPath); err != nil {
		t.Fatal(err)
	}
	if err := dxh.AddRealProgramLink(binDir, "claude", filepath.Join(fakeBinDir, "claude")); err != nil {
		t.Fatal(err)
	}

	env := dxac42Env(t, claudeDir, t.TempDir(), binDir)

	snapBefore := dxac42Snapshot(claudeDir)

	code, out := dxac42RunSetsid(ctx, bin, env, "agent", "setup", "claude-code",
		"--deny-rules", "yes")
	if code != 1 {
		t.Fatalf("DXAC42 claude-settings: expected exit 1; got %d\n%s", code, out)
	}
	dxac42AssertConflictLine(t, "claude-settings", out, otherCA, cfgDir, settings)

	snapAfter := dxac42Snapshot(claudeDir)
	dxac42AssertUnchanged(t, "claude-settings conflict", snapBefore, snapAfter)

	bundleCmd := dxac42ParseBundleCmd(out)
	if bundleCmd == "" {
		t.Fatalf("DXAC42 claude-settings: output holds no bundle command; got %q", out)
	}
	if err := dxac42RunBundleCmd(bundleCmd); err != nil {
		t.Fatalf("DXAC42 claude-settings: run bundle command: %v", err)
	}
	if _, err := os.Stat(bundlePath); err != nil {
		t.Fatalf("DXAC42 claude-settings: bundle file not created at %s: %v", bundlePath, err)
	}

	env2 := dxac42Env(t, claudeDir, t.TempDir(), binDir)
	settings2Content := `{"env":{"NODE_EXTRA_CA_CERTS":"` + bundlePath + `"}}`
	if err := os.WriteFile(settings, []byte(settings2Content), 0o640); err != nil {
		t.Fatalf("DXAC42 claude-settings: rewrite settings: %v", err)
	}
	code2, out2 := dxac42RunSetsid(ctx, bin, env2, "agent", "setup", "claude-code",
		"--deny-rules", "yes")
	if code2 != 0 {
		t.Fatalf("DXAC42 claude-settings: after bundle, expected exit 0; got %d\n%s", code2, out2)
	}
}

func dxac42Snapshot(dirs ...string) map[string][]byte {
	out := map[string][]byte{}
	for _, dir := range dirs {
		filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			data, err := os.ReadFile(path)
			if err == nil {
				out[path] = data
			}
			return nil
		})
	}
	return out
}

func dxac42AssertUnchanged(t *testing.T, label string, before, after map[string][]byte) {
	t.Helper()
	for path, bData := range before {
		aData, ok := after[path]
		if !ok {
			continue
		}
		if !bytes.Equal(bData, aData) {
			t.Errorf("DXAC42 %s: file modified: %s", label, path)
		}
	}
	for path := range after {
		if _, existed := before[path]; !existed {
			t.Errorf("DXAC42 %s: new file created: %s", label, path)
		}
	}
}

func dxac42ParseBundleCmd(output string) string {
	for _, line := range strings.Split(output, "\n") {
		_, after, ok := strings.Cut(line, "run: cat ")
		if !ok {
			continue
		}
		cmd, _, _ := strings.Cut(after, "; then")
		if strings.Contains(cmd, "ca.pem") {
			return "cat " + cmd
		}
	}
	return ""
}

func dxac42RunBundleCmd(cmd string) error {
	c := exec.Command("sh", "-c", cmd)
	out, err := c.CombinedOutput()
	if err != nil {
		return fmt.Errorf("bundle cmd %q: %w\n%s", cmd, err, out)
	}
	return nil
}

func dxac42Env(t *testing.T, claudeDir, piDir, binDir string) []string {
	t.Helper()
	var env []string
	for _, e := range os.Environ() {
		key, _, _ := strings.Cut(e, "=")
		switch key {
		case "HOME", "TMPDIR", "USER", "LANG":
			env = append(env, e)
		default:
			if strings.HasPrefix(key, "SAG_TEST_") {
				env = append(env, e)
			}
		}
	}
	env = append(env, "CLAUDE_CONFIG_DIR="+claudeDir)
	env = append(env, "PI_CODING_AGENT_DIR="+piDir)
	env = append(env, "PATH="+dxh.TestPATH(binDir))
	return env
}

func dxac42PrepareBundlePath(t *testing.T, cfgDir string) string {
	t.Helper()
	bundlePath := filepath.Join(cfgDir, "ca-bundle.pem")
	if err := os.Remove(bundlePath); err != nil && !os.IsNotExist(err) {
		t.Fatalf("DXAC42: remove old bundle: %v", err)
	}
	t.Cleanup(func() { os.Remove(bundlePath) })
	return bundlePath
}

func dxac42WriteOtherCA(t *testing.T) string {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("DXAC42: other CA key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(time.Now().UnixNano()),
		Subject:               pkix.Name{CommonName: "dxac42 other CA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("DXAC42: other CA certificate: %v", err)
	}
	path := filepath.Join(t.TempDir(), "other-ca.pem")
	data := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("DXAC42: write other CA: %v", err)
	}
	return path
}

func dxac42AssertConflictLine(t *testing.T, label, out, otherCA, cfgDir, source string) {
	t.Helper()
	caPath := filepath.Join(cfgDir, "ca.pem")
	bundlePath := filepath.Join(cfgDir, "ca-bundle.pem")
	wantFull := fmt.Sprintf(
		"fail ca-trust NODE_EXTRA_CA_CERTS is %s; run: cat %q %q > %q; then set NODE_EXTRA_CA_CERTS to %s in %s",
		otherCA, otherCA, caPath, bundlePath, bundlePath, source)
	if !strings.Contains(out, wantFull) {
		t.Errorf("DXAC42 %s: output missing full bundle line %q; got %q (waits for D6.6c B94 on pi-env case)", label, wantFull, out)
	}
	if strings.Contains(out, "container sag does not run") {
		t.Errorf("DXAC42 %s: exit 1 has another cause; got %q", label, out)
	}
}

func dxac42RunSetsid(ctx context.Context, bin string, env []string, args ...string) (int, string) {
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = env
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	runErr := cmd.Run()
	code := 0
	if runErr != nil {
		if exitErr, ok := runErr.(*exec.ExitError); ok {
			code = exitErr.ExitCode()
		}
	}
	return code, out.String()
}
