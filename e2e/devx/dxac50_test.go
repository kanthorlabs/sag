//go:build devxe2e

package devx_test

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/kanthorlabs/sag/e2e/devx/dxh"
)

const (
	fakeTailscaleIP    = "100.64.0.1"
	fakeTailscaleIPAlt = "100.64.1.2"
)

func DXAC50_TailscaleFake(t *testing.T, ctx context.Context) {
	t.Helper()

	sagBin, err := dxh.TestBinary()
	if err != nil {
		t.Fatalf("DXAC50: test binary: %v", err)
	}

	fakeBin, err := dxac50BuildFakeTailscale(t, ctx)
	if err != nil {
		t.Fatalf("DXAC50: build fake tailscale: %v", err)
	}

	podmanPath, err := exec.LookPath("podman")
	if err != nil {
		t.Fatalf("DXAC50: find podman: %v", err)
	}

	groupDir := t.TempDir()
	binDir := t.TempDir()

	if err := dxh.AddRealProgramLink(binDir, "podman", podmanPath); err != nil {
		t.Fatalf("DXAC50: link podman: %v", err)
	}

	installFake := func(t *testing.T) {
		t.Helper()
		data, err := os.ReadFile(fakeBin)
		if err != nil {
			t.Fatalf("DXAC50: read fake binary: %v", err)
		}
		if err := os.WriteFile(filepath.Join(binDir, "tailscale"), data, 0o755); err != nil {
			t.Fatalf("DXAC50: install fake binary: %v", err)
		}
	}

	removeFake := func() {
		_ = os.Remove(filepath.Join(binDir, "tailscale"))
	}

	cleanState := func() {
		for _, f := range []string{"ts_ip4", "ts_serve_status", "ts_rule", "ts_args"} {
			_ = os.Remove(filepath.Join(binDir, f))
		}
	}

	writeState := func(t *testing.T, name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(binDir, name), []byte(content), 0o600); err != nil {
			t.Fatalf("DXAC50: write %s: %v", name, err)
		}
	}

	readState := func(name string) string {
		data, _ := os.ReadFile(filepath.Join(binDir, name))
		return string(data)
	}

	countLine := func(text, want string) int {
		count := 0
		for _, line := range strings.Split(text, "\n") {
			if line == want {
				count++
			}
		}
		return count
	}

	runBS := func(t *testing.T) (stdout, stderr string, code int) {
		t.Helper()
		return dxac50RunBootstrap(t, ctx, groupDir, binDir, sagBin)
	}

	home := os.Getenv("HOME")
	cfgDir := dxh.CfgDir(home)
	r := dxh.NewRunner()

	t.Run("success", func(t *testing.T) {
		cleanState()
		installFake(t)
		writeState(t, "ts_ip4", fakeTailscaleIP)

		stdout, stderr, code := runBS(t)
		if code != 0 {
			t.Fatalf("DXAC50 success: exit %d; want 0\nstdout=%s\nstderr=%s", code, stdout, stderr)
		}
		if n := countLine(stdout, "install tailscale-serve"); n != 1 {
			t.Fatalf("DXAC50 success: install tailscale-serve appears %d times; want 1\nstdout=%q", n, stdout)
		}
		if _, err := os.Stat(filepath.Join(binDir, "ts_rule")); err != nil {
			t.Fatal("DXAC50 success: ts_rule not written")
		}
		args := readState("ts_args")
		if n := countLine(args, "serve --bg --tcp 8443 tcp://127.0.0.1:8443"); n != 1 {
			t.Fatalf("DXAC50 success: serve --bg appears %d times in ts_args; want 1\n%s", n, args)
		}
		if strings.Contains(args, "funnel") {
			t.Fatalf("DXAC50 success: funnel found in ts_args\n%s", args)
		}

		cert := dxac50ServerCert(t, cfgDir)
		dxac50CheckSANs(t, "success", cert,
			[]string{"localhost"},
			[]net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1"), net.ParseIP(fakeTailscaleIP)})

		dxac50CheckPortBinding(t, r, ctx)
	})

	t.Run("idempotent", func(t *testing.T) {
		cleanState()
		installFake(t)
		writeState(t, "ts_ip4", fakeTailscaleIP)

		stdout1, stderr1, code1 := runBS(t)
		if code1 != 0 {
			t.Fatalf("DXAC50 idempotent setup: exit %d; want 0\nstdout=%s\nstderr=%s", code1, stdout1, stderr1)
		}

		cert1 := dxac50ServerCert(t, cfgDir)
		fp1 := dxac50Fingerprint(cert1)

		stdout2, stderr2, code2 := runBS(t)
		if code2 != 0 {
			t.Fatalf("DXAC50 idempotent: exit %d; want 0\nstdout=%s\nstderr=%s", code2, stdout2, stderr2)
		}
		if n := countLine(stdout2, "ok tailscale-serve"); n != 1 {
			t.Fatalf("DXAC50 idempotent: ok tailscale-serve appears %d times; want 1\nstdout=%q", n, stdout2)
		}
		args := readState("ts_args")
		if n := countLine(args, "serve --bg --tcp 8443 tcp://127.0.0.1:8443"); n != 1 {
			t.Fatalf("DXAC50 idempotent: serve --bg appears %d times in ts_args; want 1 (first run only)\n%s", n, args)
		}
		if strings.Contains(args, "funnel") {
			t.Fatalf("DXAC50 idempotent: funnel found in ts_args\n%s", args)
		}

		cert2 := dxac50ServerCert(t, cfgDir)
		fp2 := dxac50Fingerprint(cert2)
		if fp1 != fp2 {
			t.Fatalf("DXAC50 idempotent: cert fingerprint changed")
		}
		dxac50CheckSANs(t, "idempotent", cert2,
			[]string{"localhost"},
			[]net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1"), net.ParseIP(fakeTailscaleIP)})
	})

	t.Run("skip-down", func(t *testing.T) {
		cleanState()
		installFake(t)
		writeState(t, "ts_ip4", "exit:1")

		certBefore := dxac50ServerCert(t, cfgDir)
		fpBefore := dxac50Fingerprint(certBefore)

		stdout, stderr, code := runBS(t)
		if code != 0 {
			t.Fatalf("DXAC50 skip-down: exit %d; want 0\nstdout=%s\nstderr=%s", code, stdout, stderr)
		}
		if n := countLine(stdout, "skip tailscale down; loopback only"); n != 1 {
			t.Fatalf("DXAC50 skip-down: skip line appears %d times; want 1\nstdout=%q", n, stdout)
		}
		args := readState("ts_args")
		if strings.Contains(args, "serve --bg") {
			t.Fatalf("DXAC50 skip-down: serve --bg in ts_args; want none\n%s", args)
		}

		certAfter := dxac50ServerCert(t, cfgDir)
		fpAfter := dxac50Fingerprint(certAfter)
		if fpBefore != fpAfter {
			t.Fatal("DXAC50 skip-down: cert changed; want kept")
		}
	})

	t.Run("skip-address-invalid-loopback", func(t *testing.T) {
		cleanState()
		installFake(t)
		writeState(t, "ts_ip4", "0.0.0.0")

		certBefore := dxac50ServerCert(t, cfgDir)
		fpBefore := dxac50Fingerprint(certBefore)

		stdout, stderr, code := runBS(t)
		if code != 0 {
			t.Fatalf("DXAC50 skip-address-invalid-loopback: exit %d; want 0\nstdout=%s\nstderr=%s", code, stdout, stderr)
		}
		if n := countLine(stdout, "skip tailscale address invalid; loopback only"); n != 1 {
			t.Fatalf("DXAC50 skip-address-invalid-loopback: skip line appears %d times; want 1\nstdout=%q", n, stdout)
		}

		certAfter := dxac50ServerCert(t, cfgDir)
		fpAfter := dxac50Fingerprint(certAfter)
		if fpBefore != fpAfter {
			t.Fatal("DXAC50 skip-address-invalid-loopback: cert changed; want kept")
		}
		for _, ip := range certAfter.IPAddresses {
			if ip.Equal(net.ParseIP("0.0.0.0")) {
				t.Fatal("DXAC50 skip-address-invalid-loopback: cert contains 0.0.0.0; want no such name")
			}
		}
	})

	t.Run("skip-address-invalid-private", func(t *testing.T) {
		cleanState()
		installFake(t)
		writeState(t, "ts_ip4", "192.168.1.10")

		certBefore := dxac50ServerCert(t, cfgDir)
		fpBefore := dxac50Fingerprint(certBefore)

		stdout, stderr, code := runBS(t)
		if code != 0 {
			t.Fatalf("DXAC50 skip-address-invalid-private: exit %d; want 0\nstdout=%s\nstderr=%s", code, stdout, stderr)
		}
		if n := countLine(stdout, "skip tailscale address invalid; loopback only"); n != 1 {
			t.Fatalf("DXAC50 skip-address-invalid-private: skip line appears %d times; want 1\nstdout=%q", n, stdout)
		}

		certAfter := dxac50ServerCert(t, cfgDir)
		fpAfter := dxac50Fingerprint(certAfter)
		if fpBefore != fpAfter {
			t.Fatal("DXAC50 skip-address-invalid-private: cert changed; want kept")
		}
		for _, ip := range certAfter.IPAddresses {
			if ip.Equal(net.ParseIP("192.168.1.10")) {
				t.Fatal("DXAC50 skip-address-invalid-private: cert contains 192.168.1.10; want no such name")
			}
		}
	})

	t.Run("skip-command-absent", func(t *testing.T) {
		cleanState()
		removeFake()

		stdout, stderr, code := runBS(t)
		if code != 0 {
			t.Fatalf("DXAC50 skip-command-absent: exit %d; want 0\nstdout=%s\nstderr=%s", code, stdout, stderr)
		}
		if n := countLine(stdout, "skip tailscale command absent; loopback only"); n != 1 {
			t.Fatalf("DXAC50 skip-command-absent: skip line appears %d times; want 1\nstdout=%q", n, stdout)
		}
	})

	t.Run("conflict-other-target", func(t *testing.T) {
		cleanState()
		installFake(t)
		writeState(t, "ts_ip4", fakeTailscaleIP)
		writeState(t, "ts_serve_status", `{"TCP":{"8443":{"TCPForward":"other:9090"}}}`)

		cfgSnap := dxac50SnapshotCfg(t, cfgDir)
		secSnap := dxac50SnapshotSecrets(t, r, ctx)

		stdout, stderr, code := runBS(t)
		if code != 1 {
			t.Fatalf("DXAC50 conflict-other-target: exit %d; want 1\nstdout=%s\nstderr=%s", code, stdout, stderr)
		}
		if n := countLine(stdout, "fail tailscale TCP 8443 serves another target"); n != 1 {
			t.Fatalf("DXAC50 conflict-other-target: fail line appears %d times; want 1\nstdout=%q", n, stdout)
		}
		if !strings.Contains(stderr, "sag: tailscale: TCP 8443 serves another target") {
			t.Fatalf("DXAC50 conflict-other-target: stderr missing sag error; got %q", stderr)
		}
		args := readState("ts_args")
		if strings.Contains(args, "serve --bg") {
			t.Fatalf("DXAC50 conflict-other-target: serve --bg in ts_args; want none\n%s", args)
		}
		if strings.Contains(args, "funnel") {
			t.Fatalf("DXAC50 conflict-other-target: funnel in ts_args\n%s", args)
		}

		dxac50AssertCfgUnchanged(t, "conflict-other-target", cfgDir, cfgSnap)
		dxac50AssertSecretsUnchanged(t, "conflict-other-target", r, ctx, secSnap)
	})

	t.Run("conflict-funnel", func(t *testing.T) {
		cleanState()
		installFake(t)
		writeState(t, "ts_ip4", fakeTailscaleIP)
		writeState(t, "ts_serve_status", `{"AllowFunnel":{"ts-node:8443":true}}`)

		cfgSnap := dxac50SnapshotCfg(t, cfgDir)
		secSnap := dxac50SnapshotSecrets(t, r, ctx)

		stdout, stderr, code := runBS(t)
		if code != 1 {
			t.Fatalf("DXAC50 conflict-funnel: exit %d; want 1\nstdout=%s\nstderr=%s", code, stdout, stderr)
		}
		if n := countLine(stdout, "fail tailscale funnel is on for port 8443"); n != 1 {
			t.Fatalf("DXAC50 conflict-funnel: fail line appears %d times; want 1\nstdout=%q", n, stdout)
		}
		if !strings.Contains(stderr, "sag: tailscale: funnel is on for port 8443") {
			t.Fatalf("DXAC50 conflict-funnel: stderr missing sag error; got %q", stderr)
		}
		args := readState("ts_args")
		if strings.Contains(args, "funnel") {
			t.Fatalf("DXAC50 conflict-funnel: funnel in ts_args\n%s", args)
		}

		dxac50AssertCfgUnchanged(t, "conflict-funnel", cfgDir, cfgSnap)
		dxac50AssertSecretsUnchanged(t, "conflict-funnel", r, ctx, secSnap)
	})

	t.Run("address-change", func(t *testing.T) {
		cleanState()
		installFake(t)
		writeState(t, "ts_ip4", fakeTailscaleIP)
		stdout1, stderr1, code1 := runBS(t)
		if code1 != 0 {
			t.Fatalf("DXAC50 address-change setup: exit %d; want 0\nstdout=%s\nstderr=%s", code1, stdout1, stderr1)
		}

		cleanState()
		installFake(t)
		writeState(t, "ts_ip4", fakeTailscaleIPAlt)
		stdout2, stderr2, code2 := runBS(t)
		if code2 != 0 {
			t.Fatalf("DXAC50 address-change run: exit %d; want 0\nstdout=%s\nstderr=%s", code2, stdout2, stderr2)
		}
		if !strings.Contains(stdout2, "install endpoint-cert") {
			t.Fatalf("DXAC50 address-change: cert not renewed when Tailscale IP changed\nstdout=%q", stdout2)
		}

		cert := dxac50ServerCert(t, cfgDir)
		dxac50CheckSANs(t, "address-change", cert,
			[]string{"localhost"},
			[]net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1"), net.ParseIP(fakeTailscaleIPAlt)})
	})
}

func dxac50ServerCert(t *testing.T, cfgDir string) *x509.Certificate {
	t.Helper()
	caPEM, err := os.ReadFile(filepath.Join(cfgDir, "ca.pem"))
	if err != nil {
		t.Fatalf("DXAC50: read ca.pem: %v", err)
	}
	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(caPEM)
	conn, err := tls.Dial("tcp", "127.0.0.1:8443", &tls.Config{
		RootCAs:    pool,
		ServerName: "localhost",
	})
	if err != nil {
		t.Fatalf("DXAC50: tls dial 127.0.0.1:8443: %v", err)
	}
	defer conn.Close()
	return conn.ConnectionState().PeerCertificates[0]
}

func dxac50Fingerprint(cert *x509.Certificate) string {
	h := sha256.Sum256(cert.Raw)
	return hex.EncodeToString(h[:])
}

func dxac50CheckSANs(t *testing.T, name string, cert *x509.Certificate, wantDNS []string, wantIPs []net.IP) {
	t.Helper()
	if len(cert.DNSNames) != len(wantDNS) {
		t.Fatalf("DXAC50 %s: cert DNS SANs = %v; want %v", name, cert.DNSNames, wantDNS)
	}
	for i, d := range wantDNS {
		if cert.DNSNames[i] != d {
			t.Fatalf("DXAC50 %s: cert DNS SANs = %v; want %v", name, cert.DNSNames, wantDNS)
		}
	}
	gotIPs := make([]string, len(cert.IPAddresses))
	for i, ip := range cert.IPAddresses {
		gotIPs[i] = ip.String()
	}
	wantIPStrs := make([]string, len(wantIPs))
	for i, ip := range wantIPs {
		wantIPStrs[i] = ip.String()
	}
	sort.Strings(gotIPs)
	sort.Strings(wantIPStrs)
	if strings.Join(gotIPs, ",") != strings.Join(wantIPStrs, ",") {
		t.Fatalf("DXAC50 %s: cert IP SANs = %v; want %v", name, gotIPs, wantIPStrs)
	}
}

type portBinding struct {
	HostIP   string `json:"HostIp"`
	HostPort string `json:"HostPort"`
}

func dxac50CheckPortBinding(t *testing.T, r dxh.Runner, ctx context.Context) {
	t.Helper()
	out, err := r.Output(ctx, "podman", "container", "inspect", "sag", "--format", "{{json .HostConfig.PortBindings}}")
	if err != nil {
		t.Fatalf("DXAC50: podman container inspect sag PortBindings: %v", err)
	}
	var bindings map[string][]portBinding
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(out))), &bindings); err != nil {
		t.Fatalf("DXAC50: parse PortBindings %q: %v", out, err)
	}
	if len(bindings) != 1 {
		t.Fatalf("DXAC50: port bindings count = %d; want 1; got %v", len(bindings), bindings)
	}
	entries, ok := bindings["8443/tcp"]
	if !ok {
		t.Fatalf("DXAC50: port binding 8443/tcp not found; got %v", bindings)
	}
	if len(entries) != 1 || entries[0].HostIP != "127.0.0.1" || entries[0].HostPort != "8443" {
		t.Fatalf("DXAC50: port binding 8443/tcp = %v; want [{HostIp:127.0.0.1 HostPort:8443}]", entries)
	}
}

func dxac50SnapshotCfg(t *testing.T, cfgDir string) map[string]string {
	t.Helper()
	hashes := map[string]string{}
	_ = filepath.WalkDir(cfgDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(cfgDir, path)
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		h := sha256.Sum256(data)
		hashes[rel] = hex.EncodeToString(h[:])
		return nil
	})
	return hashes
}

func dxac50SnapshotSecrets(t *testing.T, r dxh.Runner, ctx context.Context) []string {
	t.Helper()
	out, err := r.Output(ctx, "podman", "secret", "ls", "--format", "{{.Name}}")
	if err != nil {
		t.Fatalf("DXAC50: podman secret ls: %v", err)
	}
	var names []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line != "" {
			names = append(names, line)
		}
	}
	sort.Strings(names)
	return names
}

func dxac50AssertCfgUnchanged(t *testing.T, name, cfgDir string, before map[string]string) {
	t.Helper()
	after := dxac50SnapshotCfg(t, cfgDir)
	for k, v := range before {
		if after[k] != v {
			t.Fatalf("DXAC50 %s: cfg file %q changed after conflict", name, k)
		}
	}
	for k := range after {
		if _, ok := before[k]; !ok {
			t.Fatalf("DXAC50 %s: new cfg file %q after conflict", name, k)
		}
	}
}

func dxac50AssertSecretsUnchanged(t *testing.T, name string, r dxh.Runner, ctx context.Context, before []string) {
	t.Helper()
	after := dxac50SnapshotSecrets(t, r, ctx)
	if strings.Join(before, ",") != strings.Join(after, ",") {
		t.Fatalf("DXAC50 %s: secrets changed after conflict; before=%v after=%v", name, before, after)
	}
}

func dxac50BuildFakeTailscale(t *testing.T, ctx context.Context) (string, error) {
	t.Helper()
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", fmt.Errorf("git rev-parse: %w", err)
	}
	repoRoot := strings.TrimSpace(string(out))
	binPath := filepath.Join(t.TempDir(), "tailscale")
	buildCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	build := exec.CommandContext(buildCtx, "go", "build", "-o", binPath, "./e2e/devx/cmd/faketailscale")
	build.Dir = repoRoot
	if buildOut, err := build.CombinedOutput(); err != nil {
		return "", fmt.Errorf("go build: %w: %s", err, buildOut)
	}
	return binPath, nil
}

func dxac50RunBootstrap(t *testing.T, ctx context.Context, groupDir, binDir, sagBin string) (string, string, int) {
	t.Helper()
	env, err := dxh.ChildEnv(os.Environ(), groupDir)
	if err != nil {
		t.Fatalf("DXAC50: child env: %v", err)
	}
	env = append(env, "PATH="+dxh.TestPATH(binDir))

	testCtx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(testCtx, sagBin, "bootstrap",
		"--autostart", "no",
		"--agent-setup", "claude-code=no",
		"--agent-setup", "pi=no")
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
			t.Fatalf("DXAC50: run sag bootstrap: %v", runErr)
		}
	}
	return stdoutBuf.String(), stderrBuf.String(), code
}
