//go:build linux

package service

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

type nopRunner struct{}

func (nopRunner) Run(_ context.Context, _ io.Reader, _ ...string) ([]byte, error) {
	return nil, nil
}

func writeSAGYAML(t *testing.T, dir string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "sag.yaml"), []byte(minimalSAGYAML), 0o644); err != nil {
		t.Fatal(err)
	}
}

type fakeSystemctl struct {
	calls [][]string
	errs  map[string]error
}

func newFakeSC() *fakeSystemctl {
	return &fakeSystemctl{errs: map[string]error{}}
}

func (f *fakeSystemctl) run(_ context.Context, args ...string) ([]byte, error) {
	f.calls = append(f.calls, args)
	key := strings.Join(args, " ")
	if err, ok := f.errs[key]; ok {
		return nil, err
	}
	return nil, nil
}

func (f *fakeSystemctl) order() []string {
	var out []string
	for _, c := range f.calls {
		out = append(out, strings.Join(c, " "))
	}
	return out
}

func (f *fakeSystemctl) called(substr string) bool {
	for _, c := range f.calls {
		if strings.Contains(strings.Join(c, " "), substr) {
			return true
		}
	}
	return false
}

func testLinuxDeps(sc *fakeSystemctl) linuxDeps {
	return linuxDeps{
		systemctlRun: sc.run,
		healthzCheck: func(_ context.Context, _ string) error { return nil },
		logTail:      func(_ context.Context, _ int) ([]byte, error) { return nil, nil },
		interval:     time.Millisecond,
		timeout:      50 * time.Millisecond,
	}
}

func writerOf(sb *strings.Builder) stepline.Writer {
	return stepline.New(sb)
}

func TestLinuxStartOrder(t *testing.T) {
	dir := t.TempDir()
	writeSAGYAML(t, dir)

	sc := newFakeSC()
	d := testLinuxDeps(sc)

	home := t.TempDir()
	cfg := Config{
		Podman: nopRunner{},
		CfgDir: dir,
		UID:    "1000",
		Home:   home,
		Image:  "localhost/sag:test",
	}

	var out strings.Builder
	if err := start(context.Background(), cfg, d, writerOf(&out)); err != nil {
		t.Fatal(err)
	}

	order := sc.order()
	reloadIdx := -1
	resetIdx := -1
	restartIdx := -1
	for i, c := range order {
		if strings.Contains(c, "daemon-reload") {
			reloadIdx = i
		}
		if strings.Contains(c, "reset-failed sag") {
			resetIdx = i
		}
		if strings.Contains(c, "restart sag") {
			restartIdx = i
		}
	}
	if reloadIdx < 0 {
		t.Fatal("daemon-reload was not called")
	}
	if restartIdx < 0 {
		t.Fatal("restart sag was not called")
	}
	if reloadIdx >= restartIdx {
		t.Fatalf("daemon-reload (call[%d]) must come before restart sag (call[%d])", reloadIdx, restartIdx)
	}
	if resetIdx < reloadIdx || resetIdx >= restartIdx {
		t.Fatalf("reset-failed sag (call[%d]) must come after daemon-reload (call[%d]) and before restart sag (call[%d])", resetIdx, reloadIdx, restartIdx)
	}
}

func TestStartAutostartFromAnswers(t *testing.T) {
	dir := t.TempDir()
	writeSAGYAML(t, dir)
	if err := os.WriteFile(filepath.Join(dir, "answers.json"), []byte(`{"autostart":"yes"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	sc := newFakeSC()
	d := testLinuxDeps(sc)

	home := t.TempDir()
	cfg := Config{
		Podman: nopRunner{},
		CfgDir: dir,
		UID:    "1000",
		Home:   home,
		Image:  "localhost/sag:test",
	}

	var out strings.Builder
	if err := start(context.Background(), cfg, d, writerOf(&out)); err != nil {
		t.Fatal(err)
	}

	content, err := os.ReadFile(quadletPath(home))
	if err != nil {
		t.Fatalf("read quadlet: %v", err)
	}
	if !strings.Contains(string(content), "[Install]") || !strings.Contains(string(content), "WantedBy=default.target") {
		t.Fatal("Quadlet missing [Install]/WantedBy=default.target when answers.json autostart=yes")
	}
}

func TestQuadletAutostartNo(t *testing.T) {
	dir := t.TempDir()
	writeSAGYAML(t, dir)

	sc := newFakeSC()
	d := testLinuxDeps(sc)

	home := t.TempDir()
	cfg := Config{
		Podman: nopRunner{},
		CfgDir: dir,
		UID:    "1000",
		Home:   home,
		Image:  "localhost/sag:test",
	}

	var out strings.Builder
	changed, err := applyAutostart(context.Background(), cfg, false, d, writerOf(&out))
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Error("expected changed=true on first write")
	}

	content, err := os.ReadFile(quadletPath(home))
	if err != nil {
		t.Fatalf("read quadlet: %v", err)
	}
	if strings.Contains(string(content), "[Install]") {
		t.Fatal("Quadlet has [Install] section with autostart=no")
	}
}

func TestQuadletAutostartYes(t *testing.T) {
	dir := t.TempDir()
	writeSAGYAML(t, dir)

	sc := newFakeSC()
	d := testLinuxDeps(sc)

	home := t.TempDir()
	cfg := Config{
		Podman: nopRunner{},
		CfgDir: dir,
		UID:    "1000",
		Home:   home,
		Image:  "localhost/sag:test",
	}

	var out strings.Builder
	changed, err := applyAutostart(context.Background(), cfg, true, d, writerOf(&out))
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Error("expected changed=true on first write")
	}

	content, err := os.ReadFile(quadletPath(home))
	if err != nil {
		t.Fatalf("read quadlet: %v", err)
	}
	if !strings.Contains(string(content), "[Install]") {
		t.Fatal("Quadlet missing [Install] section with autostart=yes")
	}
	if !strings.Contains(string(content), "WantedBy=default.target") {
		t.Fatal("Quadlet missing WantedBy=default.target")
	}
}

func TestStopUsesSystemctl(t *testing.T) {
	sc := newFakeSC()
	d := linuxDeps{
		systemctlRun: sc.run,
		interval:     time.Millisecond,
		timeout:      time.Millisecond,
	}

	cfg := Config{
		Podman: nopRunner{},
		CfgDir: t.TempDir(),
		UID:    "1000",
		Home:   t.TempDir(),
		Image:  "localhost/sag:test",
	}

	var out strings.Builder
	if err := stop(context.Background(), cfg, d, writerOf(&out)); err != nil {
		t.Fatal(err)
	}

	if !sc.called("--user stop sag") {
		t.Fatal("systemctl --user stop sag was not called")
	}
}
