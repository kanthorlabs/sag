//go:build darwin

package service

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kanthorlabs/sag/internal/podman"
	"github.com/kanthorlabs/sag/internal/stepline"
)

const minimalSAGYAML = `version: 1
server:
  listen: 127.0.0.1:8443
  tls_cert: "secret:sag-tls-cert"
  tls_key: "secret:sag-tls-key"
auth:
  version: 1
  token_lifetime: 8760h
`

type fakeRunner struct {
	calls    []string
	outputs  map[string][]byte
	errors   map[string]error
	defaults []byte
}

func newFake() *fakeRunner {
	return &fakeRunner{
		outputs: make(map[string][]byte),
		errors:  make(map[string]error),
	}
}

func (f *fakeRunner) Run(_ context.Context, _ io.Reader, args ...string) ([]byte, error) {
	key := strings.Join(args, " ")
	f.calls = append(f.calls, key)
	if err, ok := f.errors[key]; ok {
		return nil, err
	}
	if out, ok := f.outputs[key]; ok {
		return out, nil
	}
	return f.defaults, nil
}

func (f *fakeRunner) called(substr string) bool {
	for _, c := range f.calls {
		if strings.Contains(c, substr) {
			return true
		}
	}
	return false
}

func writerOf(sb *strings.Builder) stepline.Writer {
	return stepline.New(sb)
}

func writeSAGYAML(t *testing.T, dir string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "sag.yaml"), []byte(minimalSAGYAML), 0o644); err != nil {
		t.Fatal(err)
	}
}

func testDeps(healthzErr error) deps {
	return deps{
		launchctlRun: func(_ context.Context, _ ...string) ([]byte, error) { return nil, nil },
		healthzCheck: func(_ context.Context, _ string) error { return healthzErr },
		logTail:      func(_ context.Context, _ int) ([]byte, error) { return nil, nil },
		interval:     time.Millisecond,
		timeout:      50 * time.Millisecond,
	}
}

func TestMacStartsStoppedMachineOnly(t *testing.T) {
	dir := t.TempDir()
	writeSAGYAML(t, dir)

	t.Run("running machine does not start", func(t *testing.T) {
		r := newFake()
		r.outputs["machine list --format json"] = []byte(`[{"Name":"podman-machine-default","Running":true}]`)

		cfg := Config{
			Podman: r, CfgDir: dir, UID: "501", Home: dir, BinPath: "/usr/local/bin/sag",
			Image: "localhost/sag:test",
		}
		d := testDeps(nil)
		d.healthzCheck = func(_ context.Context, _ string) error { return nil }

		var out strings.Builder
		if err := start(context.Background(), cfg, d, writerOf(&out)); err != nil {
			t.Fatal(err)
		}
		if r.called("machine start") {
			t.Fatal("machine start called on running machine")
		}
		if !r.called("--replace") {
			t.Fatal("podman run did not include --replace")
		}
		if !r.called("--restart=on-failure") {
			t.Fatal("podman run did not include --restart=on-failure")
		}
		if !r.called("localhost/sag:test") {
			t.Fatal("podman run did not use the configured image")
		}
	})

	t.Run("stopped machine is started", func(t *testing.T) {
		r := newFake()
		r.outputs["machine list --format json"] = []byte(`[{"Name":"podman-machine-default","Running":false}]`)

		cfg := Config{
			Podman: r, CfgDir: dir, UID: "501", Home: dir, BinPath: "/usr/local/bin/sag",
			Image: "localhost/sag:test",
		}
		d := testDeps(nil)
		d.healthzCheck = func(_ context.Context, _ string) error { return nil }

		var out strings.Builder
		_ = start(context.Background(), cfg, d, writerOf(&out))
		if !r.called("machine start") {
			t.Fatal("machine start not called for stopped machine")
		}
	})
}

func TestHealthzTimeoutTailsLog(t *testing.T) {
	dir := t.TempDir()
	writeSAGYAML(t, dir)

	r := newFake()
	r.outputs["machine list --format json"] = []byte(`[{"Name":"podman-machine-default","Running":true}]`)

	cfg := Config{
		Podman: r, CfgDir: dir, UID: "501", Home: dir, BinPath: "/usr/local/bin/sag",
		Image: "localhost/sag:test",
	}
	var logTailN int
	d := deps{
		launchctlRun: func(_ context.Context, _ ...string) ([]byte, error) { return nil, nil },
		healthzCheck: func(_ context.Context, _ string) error { return fmt.Errorf("not ready") },
		logTail: func(_ context.Context, n int) ([]byte, error) {
			logTailN = n
			return []byte("container log line"), nil
		},
		interval: time.Millisecond,
		timeout:  10 * time.Millisecond,
	}

	var out strings.Builder
	err := start(context.Background(), cfg, d, writerOf(&out))
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !strings.Contains(err.Error(), "container log line") {
		t.Fatalf("error %q does not contain log tail", err.Error())
	}
	if logTailN != logTailLines {
		t.Fatalf("logTail called with n=%d; want %d", logTailN, logTailLines)
	}
}

func TestLaunchAgentPlist(t *testing.T) {
	binPath := "/usr/local/bin/sag"
	logDir := "/Users/test/Library/Logs/sag"
	plist := buildPlist(binPath, logDir)

	if !strings.Contains(plist, "net.kanthorlabs.sag") {
		t.Error("plist missing label")
	}
	if !strings.Contains(plist, binPath) {
		t.Error("plist missing binPath")
	}
	if !strings.Contains(plist, "service") {
		t.Error("plist missing 'service' argument")
	}
	if !strings.Contains(plist, "start") {
		t.Error("plist missing 'start' argument")
	}
	if !strings.Contains(plist, filepath.Join(logDir, "service.log")) {
		t.Error("plist missing log path")
	}

	type plistDict struct {
		Keys   []string `xml:"key"`
		Values []string `xml:"true"`
	}
	plistStr := plist
	runAtLoadIdx := strings.Index(plistStr, "<key>RunAtLoad</key>")
	if runAtLoadIdx < 0 {
		t.Fatal("plist missing RunAtLoad key")
	}
	afterKey := plistStr[runAtLoadIdx+len("<key>RunAtLoad</key>"):]
	trimmed := strings.TrimSpace(afterKey)
	if !strings.HasPrefix(trimmed, "<true/>") {
		t.Fatalf("RunAtLoad value is not <true/>; got %q", trimmed[:min(len(trimmed), 20)])
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func TestBootoutNotLoadedIsNotFailure(t *testing.T) {
	dir := t.TempDir()
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, "Library", "LaunchAgents"), 0o755); err != nil {
		t.Fatal(err)
	}

	bootoutCalled := false
	d := deps{
		launchctlRun: func(_ context.Context, args ...string) ([]byte, error) {
			if len(args) > 0 && args[0] == "bootout" {
				bootoutCalled = true
				return nil, &launchctlError{code: 36, stderr: "Could not find specified service"}
			}
			return nil, nil
		},
		logTail:  func(_ context.Context, _ int) ([]byte, error) { return nil, nil },
		interval: time.Millisecond,
		timeout:  time.Millisecond,
	}

	cfg := Config{
		Podman: newFake(), CfgDir: dir, UID: "501", Home: home, BinPath: "/usr/local/bin/sag",
		Image: "localhost/sag:test",
	}

	var out strings.Builder
	changed, err := applyAutostart(context.Background(), cfg, true, d, writerOf(&out))
	if err != nil {
		t.Fatalf("applyAutostart with not-loaded bootout: %v", err)
	}
	if !changed {
		t.Error("expected changed=true")
	}
	if !bootoutCalled {
		t.Error("bootout was not called")
	}
}

func TestUnchangedPlistNoReload(t *testing.T) {
	dir := t.TempDir()
	home := t.TempDir()
	laDir := filepath.Join(home, "Library", "LaunchAgents")
	logDir := filepath.Join(home, "Library", "Logs", "sag")
	if err := os.MkdirAll(laDir, 0o755); err != nil {
		t.Fatal(err)
	}

	plist := buildPlist("/usr/local/bin/sag", logDir)
	plistPath := filepath.Join(laDir, launchAgentLabel+".plist")
	if err := os.WriteFile(plistPath, []byte(plist), 0o644); err != nil {
		t.Fatal(err)
	}

	var launchctlCalls []string
	d := deps{
		launchctlRun: func(_ context.Context, args ...string) ([]byte, error) {
			if len(args) > 0 {
				launchctlCalls = append(launchctlCalls, args[0])
			}
			return nil, nil
		},
		logTail:  func(_ context.Context, _ int) ([]byte, error) { return nil, nil },
		interval: time.Millisecond,
		timeout:  time.Millisecond,
	}

	cfg := Config{
		Podman: newFake(), CfgDir: dir, UID: "501", Home: home, BinPath: "/usr/local/bin/sag",
		Image: "localhost/sag:test",
	}

	var out strings.Builder
	changed, err := applyAutostart(context.Background(), cfg, true, d, writerOf(&out))
	if err != nil {
		t.Fatalf("applyAutostart: %v", err)
	}
	if changed {
		t.Error("expected changed=false when plist unchanged")
	}
	if len(launchctlCalls) > 0 {
		t.Errorf("launchctl called when plist unchanged: %v", launchctlCalls)
	}
	if !strings.Contains(out.String(), "ok autostart") {
		t.Errorf("expected ok autostart line, got: %q", out.String())
	}
}

func TestNoGUIDomainWarns(t *testing.T) {
	dir := t.TempDir()
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, "Library", "LaunchAgents"), 0o755); err != nil {
		t.Fatal(err)
	}

	d := deps{
		launchctlRun: func(_ context.Context, args ...string) ([]byte, error) {
			if len(args) > 0 && args[0] == "print" {
				return nil, fmt.Errorf("could not find domain")
			}
			return nil, nil
		},
		logTail:  func(_ context.Context, _ int) ([]byte, error) { return nil, nil },
		interval: time.Millisecond,
		timeout:  time.Millisecond,
	}

	cfg := Config{
		Podman: newFake(), CfgDir: dir, UID: "501", Home: home, BinPath: "/usr/local/bin/sag",
		Image: "localhost/sag:test",
	}

	var out strings.Builder
	changed, err := applyAutostart(context.Background(), cfg, true, d, writerOf(&out))
	if err != nil {
		t.Fatalf("applyAutostart: %v", err)
	}
	if !changed {
		t.Error("expected changed=true")
	}
	if !strings.Contains(out.String(), "warn autostart") {
		t.Errorf("expected warn autostart line, got: %q", out.String())
	}
	plistPath := filepath.Join(home, "Library", "LaunchAgents", launchAgentLabel+".plist")
	if _, err := os.Stat(plistPath); err != nil {
		t.Errorf("plist not written: %v", err)
	}
}

func TestAutostartDisableRemovesPlist(t *testing.T) {
	dir := t.TempDir()
	home := t.TempDir()
	laDir := filepath.Join(home, "Library", "LaunchAgents")
	logDir := filepath.Join(home, "Library", "Logs", "sag")
	if err := os.MkdirAll(laDir, 0o755); err != nil {
		t.Fatal(err)
	}

	plist := buildPlist("/usr/local/bin/sag", logDir)
	plistPath := filepath.Join(laDir, launchAgentLabel+".plist")
	if err := os.WriteFile(plistPath, []byte(plist), 0o644); err != nil {
		t.Fatal(err)
	}

	bootoutCalled := false
	d := deps{
		launchctlRun: func(_ context.Context, args ...string) ([]byte, error) {
			if len(args) > 0 && args[0] == "bootout" {
				bootoutCalled = true
			}
			return nil, nil
		},
		logTail:  func(_ context.Context, _ int) ([]byte, error) { return nil, nil },
		interval: time.Millisecond,
		timeout:  time.Millisecond,
	}

	cfg := Config{
		Podman: newFake(), CfgDir: dir, UID: "501", Home: home, BinPath: "/usr/local/bin/sag",
		Image: "localhost/sag:test",
	}

	var out strings.Builder
	changed, err := applyAutostart(context.Background(), cfg, false, d, writerOf(&out))
	if err != nil {
		t.Fatalf("applyAutostart enable=false: %v", err)
	}
	if !bootoutCalled {
		t.Error("bootout was not called on disable")
	}
	if _, err := os.Stat(plistPath); err == nil {
		t.Error("plist still exists after disable; want removed")
	}
	if !changed {
		t.Error("expected changed=true when plist was removed")
	}
	if !strings.Contains(out.String(), "install autostart") {
		t.Errorf("expected install autostart line, got: %q", out.String())
	}
}

func TestLinuxUnsupported(t *testing.T) {
	err := unsupportedPlatformErr("linux", "amd64")
	if err == nil {
		t.Fatal("expected non-nil error")
	}
	if err.Error() != "unsupported platform linux/amd64" {
		t.Fatalf("error = %q; want %q", err.Error(), "unsupported platform linux/amd64")
	}
	if CheckPlatform("linux", "amd64") == nil {
		t.Fatal("CheckPlatform(linux, amd64) = nil; want error")
	}
	if err2 := CheckPlatform("darwin", "arm64"); err2 != nil {
		t.Fatalf("CheckPlatform(darwin, arm64) = %v; want nil", err2)
	}
}

type countingSSH struct {
	fakeRunner
	failures int
	sshCalls int
	order    []string
}

func (c *countingSSH) Run(ctx context.Context, stdin io.Reader, args ...string) ([]byte, error) {
	key := strings.Join(args, " ")
	if strings.HasPrefix(key, "machine ssh -- test -f ") {
		c.sshCalls++
		c.order = append(c.order, "ssh")
		if c.sshCalls <= c.failures {
			return nil, fmt.Errorf("exit status 1")
		}
		return nil, nil
	}
	if len(args) > 0 && args[0] == "run" {
		c.order = append(c.order, "run")
	}
	return c.fakeRunner.Run(ctx, stdin, args...)
}

func TestWaitMachineSeesRetriesUntilVisible(t *testing.T) {
	c := &countingSSH{fakeRunner: *newFake(), failures: 2}
	if err := waitMachineSees(context.Background(), c, "/x/sag.yaml", time.Second, time.Millisecond); err != nil {
		t.Fatalf("waitMachineSees: %v", err)
	}
	if c.sshCalls != 3 {
		t.Fatalf("ssh calls = %d; want 3", c.sshCalls)
	}
}

func TestWaitMachineSeesTimesOut(t *testing.T) {
	c := &countingSSH{fakeRunner: *newFake(), failures: 1 << 30}
	err := waitMachineSees(context.Background(), c, "/x/sag.yaml", 20*time.Millisecond, time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "/x/sag.yaml") {
		t.Fatalf("waitMachineSees: err = %v; want a timeout that names the path", err)
	}
}

func TestStartWaitsForConfigBeforeRun(t *testing.T) {
	dir := t.TempDir()
	writeSAGYAML(t, dir)
	c := &countingSSH{fakeRunner: *newFake(), failures: 1}
	c.outputs["machine list --format json"] = []byte(`[{"Name":"podman-machine-default","Running":true}]`)
	d := testDeps(nil)
	d.configVisible = func(ctx context.Context, pm podman.Command, path string) error {
		return waitMachineSees(ctx, pm, path, time.Second, time.Millisecond)
	}
	var sb strings.Builder
	if err := start(context.Background(), Config{CfgDir: dir, Podman: c, Image: "img"}, d, writerOf(&sb)); err != nil {
		t.Fatalf("start: %v", err)
	}
	want := []string{"ssh", "ssh", "run"}
	if strings.Join(c.order, ",") != strings.Join(want, ",") {
		t.Fatalf("order = %v; want %v", c.order, want)
	}
}

func TestStartStopsWhenConfigInvisible(t *testing.T) {
	dir := t.TempDir()
	writeSAGYAML(t, dir)
	c := &countingSSH{fakeRunner: *newFake(), failures: 1 << 30}
	c.outputs["machine list --format json"] = []byte(`[{"Name":"podman-machine-default","Running":true}]`)
	d := testDeps(nil)
	d.configVisible = func(ctx context.Context, pm podman.Command, path string) error {
		return waitMachineSees(ctx, pm, path, 20*time.Millisecond, time.Millisecond)
	}
	var sb strings.Builder
	if err := start(context.Background(), Config{CfgDir: dir, Podman: c, Image: "img"}, d, writerOf(&sb)); err == nil {
		t.Fatal("start: want error when the machine does not see sag.yaml")
	}
	for _, o := range c.order {
		if o == "run" {
			t.Fatal("start ran podman run although the machine does not see sag.yaml")
		}
	}
}
