package bootstrap

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/kanthorlabs/sag/internal/config"
	"github.com/kanthorlabs/sag/internal/endpointcert"
	"github.com/kanthorlabs/sag/internal/hoststate"
	"github.com/kanthorlabs/sag/internal/podman"
	"github.com/kanthorlabs/sag/internal/prompt"
	"github.com/kanthorlabs/sag/internal/service"
	"github.com/kanthorlabs/sag/internal/stepline"
)

type nopPodman struct{}

func (nopPodman) Run(_ context.Context, _ io.Reader, _ ...string) ([]byte, error) {
	return nil, nil
}

func baseDeps(dir string) Deps {
	cfgDir := filepath.Join(dir, "sag")
	out := stepline.New(io.Discard)
	return Deps{
		GOOS:   func() string { return "darwin" },
		GOARCH: func() string { return "arm64" },
		OpenFile: func(path string) error {
			return &fs.PathError{Op: "open", Path: path, Err: fs.ErrNotExist}
		},
		UID:     func() string { return "501" },
		Home:    func() string { return dir },
		SAGPath: func() (string, error) { return "/usr/local/bin/sag", nil },
		ResolvePaths: func() (hoststate.Paths, error) {
			return hoststate.Paths{
				Cfg:         cfgDir,
				SAGYaml:     filepath.Join(cfgDir, "sag.yaml"),
				CAPem:       filepath.Join(cfgDir, "ca.pem"),
				AnswersJSON: filepath.Join(cfgDir, "answers.json"),
				Lock:        filepath.Join(cfgDir, "lock"),
			}, nil
		},
		EnsureCfg: func(p hoststate.Paths) (bool, error) {
			if _, err := os.Stat(p.Cfg); err == nil {
				return false, nil
			}
			return true, os.MkdirAll(p.Cfg, 0o700)
		},
		AcquireLock: func(p hoststate.Paths) (*hoststate.Lock, error) {
			return &hoststate.Lock{}, nil
		},
		ReleaseLock: func(l *hoststate.Lock) error { return nil },
		LoadAnswers: func(p hoststate.Paths) (hoststate.Answers, error) {
			return hoststate.Answers{}, fs.ErrNotExist
		},
		SaveAnswers: func(p hoststate.Paths, a hoststate.Answers) error { return nil },
		FindPodman:  func(stored string) (string, error) { return "/usr/bin/podman", nil },
		ResolveYesNo: func(a prompt.Asker, flagValue, stored, question, flagName string) (string, error) {
			if flagValue != "" {
				return flagValue, nil
			}
			if stored != "" {
				return stored, nil
			}
			return "", &prompt.RefusalError{Flag: flagName}
		},
		Asker:     prompt.Asker{},
		PodmanCmd: func(path string) podman.Command { return nopPodman{} },
		ImageRef:  func() string { return "docker.io/kanthorlabs/sag:main" },
		ImageEnsure: func(ctx context.Context, pm podman.Command, ref string, w stepline.Writer) (bool, error) {
			return false, nil
		},
		MasterKey: func(ctx context.Context, pm podman.Command, w stepline.Writer) (bool, error) {
			return false, nil
		},
		CertEnsure: func(ctx context.Context, pm podman.Command, cfg endpointcert.Config, now time.Time, w stepline.Writer) (bool, error) {
			return false, nil
		},
		SAGYAMLCreate: func(cfgDir string) (bool, error) { return false, nil },
		ApplyAutostart: func(ctx context.Context, cfg service.Config, enable bool, w stepline.Writer) (bool, error) {
			return false, nil
		},
		ServiceRunning: func(ctx context.Context, cfg service.Config) (bool, error) { return true, nil },
		ServiceStart:   func(ctx context.Context, cfg service.Config, w stepline.Writer) error { return nil },
		CheckHealthz:   func(ctx context.Context, cfgDir string) error { return nil },
		Now:            func() time.Time { return time.Time{} },
		Out:            out,
		Print:          io.Discard,
	}
}

func TestStepOrder(t *testing.T) {
	dir := t.TempDir()
	deps := baseDeps(dir)

	var ranSteps []string
	deps.ImageEnsure = func(ctx context.Context, pm podman.Command, ref string, w stepline.Writer) (bool, error) {
		ranSteps = append(ranSteps, "image")
		return false, nil
	}
	deps.MasterKey = func(ctx context.Context, pm podman.Command, w stepline.Writer) (bool, error) {
		ranSteps = append(ranSteps, "master-key")
		return false, nil
	}
	deps.CertEnsure = func(ctx context.Context, pm podman.Command, cfg endpointcert.Config, now time.Time, w stepline.Writer) (bool, error) {
		ranSteps = append(ranSteps, "cert")
		return false, nil
	}
	deps.SAGYAMLCreate = func(cfgDir string) (bool, error) {
		ranSteps = append(ranSteps, "sag-yaml")
		return false, nil
	}
	deps.ApplyAutostart = func(ctx context.Context, cfg service.Config, enable bool, w stepline.Writer) (bool, error) {
		ranSteps = append(ranSteps, "autostart")
		return false, nil
	}

	if err := Run(context.Background(), Flags{Autostart: "yes"}, deps); err != nil {
		t.Fatalf("Run: %v", err)
	}

	built := deps.BuildSteps()
	if len(built) != len(Steps) {
		t.Fatalf("BuildSteps() len = %d; want %d", len(built), len(Steps))
	}
	for i, s := range Steps {
		if built[i].Name != s.Name {
			t.Fatalf("BuildSteps()[%d].Name = %q; want %q", i, built[i].Name, s.Name)
		}
	}

	wantSubOrder := []string{"image", "master-key", "cert", "sag-yaml", "autostart"}
	j := 0
	for _, got := range ranSteps {
		if j < len(wantSubOrder) && got == wantSubOrder[j] {
			j++
		}
	}
	if j != len(wantSubOrder) {
		t.Fatalf("step sub-order: ran %v; want %v in order", ranSteps, wantSubOrder)
	}
}

func TestUnsupportedPlatformExit1(t *testing.T) {
	deps := Deps{
		GOOS:   func() string { return "linux" },
		GOARCH: func() string { return "amd64" },
		Out:    stepline.New(io.Discard),
		Print:  io.Discard,
	}
	steps := deps.BuildSteps()
	s := &State{Out: deps.Out}
	err := steps[0].Run(context.Background(), s)
	if err == nil {
		t.Fatal("step platform-check: want error, got nil")
	}
	if err.Error() != "unsupported platform linux/amd64" {
		t.Fatalf("step platform-check error = %q; want %q", err.Error(), "unsupported platform linux/amd64")
	}
}

func TestExitInPlanLeavesNoCfg(t *testing.T) {
	dir := t.TempDir()
	cfgDir := filepath.Join(dir, "sag")

	deps := baseDeps(dir)
	deps.ResolveYesNo = func(a prompt.Asker, flagValue, stored, question, flagName string) (string, error) {
		return "", &prompt.RefusalError{Flag: flagName}
	}

	err := Run(context.Background(), Flags{}, deps)
	if err == nil {
		t.Fatal("Run: want error, got nil")
	}
	var refusal *prompt.RefusalError
	if !errors.As(err, &refusal) {
		t.Fatalf("Run error type = %T; want *prompt.RefusalError", err)
	}

	if _, statErr := os.Stat(cfgDir); statErr == nil {
		t.Fatalf("cfg dir %s exists after exit in plan; want absent", cfgDir)
	}
}

func TestPlanTwice(t *testing.T) {
	dir := t.TempDir()

	type call struct{ kind string }
	var calls []call

	deps := baseDeps(dir)
	deps.LoadAnswers = func(p hoststate.Paths) (hoststate.Answers, error) {
		calls = append(calls, call{"load"})
		return hoststate.Answers{}, fs.ErrNotExist
	}
	deps.EnsureCfg = func(p hoststate.Paths) (bool, error) {
		calls = append(calls, call{"ensure-cfg"})
		if _, err := os.Stat(p.Cfg); err == nil {
			return false, nil
		}
		return true, os.MkdirAll(p.Cfg, 0o700)
	}
	deps.AcquireLock = func(p hoststate.Paths) (*hoststate.Lock, error) {
		calls = append(calls, call{"lock"})
		return &hoststate.Lock{}, nil
	}
	promptCalls := 0
	deps.ResolveYesNo = func(a prompt.Asker, flagValue, stored, question, flagName string) (string, error) {
		if flagValue != "" {
			return flagValue, nil
		}
		if stored != "" {
			return stored, nil
		}
		promptCalls++
		return "yes", nil
	}

	var outBuf bytes.Buffer
	deps.Out = stepline.New(&outBuf)

	if err := Run(context.Background(), Flags{}, deps); err != nil {
		t.Fatalf("Run: unexpected error: %v", err)
	}

	if promptCalls != 1 {
		t.Fatalf("prompt called %d times; want 1", promptCalls)
	}

	wantOrder := []string{"load", "ensure-cfg", "lock", "load"}
	for i, want := range wantOrder {
		if i >= len(calls) {
			t.Fatalf("calls[%d] missing; want %q", i, want)
		}
		if calls[i].kind != want {
			t.Fatalf("calls[%d] = %q; want %q", i, calls[i].kind, want)
		}
	}
}

func TestSAGConfigTemplate(t *testing.T) {
	dir := t.TempDir()
	created, err := createSAGConfig(dir)
	if err != nil {
		t.Fatalf("createSAGConfig: %v", err)
	}
	if !created {
		t.Fatal("createSAGConfig: want created=true, got false")
	}

	_, err = config.Load(filepath.Join(dir, "sag.yaml"))
	if err != nil {
		t.Fatalf("config.Load after createSAGConfig: %v", err)
	}

	created2, err := createSAGConfig(dir)
	if err != nil {
		t.Fatalf("createSAGConfig (second): %v", err)
	}
	if created2 {
		t.Fatal("createSAGConfig (second): want created=false, got true")
	}
}

func TestSaveAnswersPreservesExistingKeys(t *testing.T) {
	dir := t.TempDir()
	cfgDir := filepath.Join(dir, "sag")

	var saved hoststate.Answers
	deps := baseDeps(dir)
	deps.LoadAnswers = func(p hoststate.Paths) (hoststate.Answers, error) {
		return hoststate.Answers{
			AgentSetup:  map[string]string{"claude-code": "yes"},
			TailscaleIP: "100.64.0.1",
		}, nil
	}
	deps.SaveAnswers = func(p hoststate.Paths, a hoststate.Answers) error {
		saved = a
		return nil
	}
	deps.ResolveYesNo = func(a prompt.Asker, flagValue, stored, question, flagName string) (string, error) {
		return "yes", nil
	}

	if err := Run(context.Background(), Flags{Autostart: "yes"}, deps); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if saved.AgentSetup["claude-code"] != "yes" {
		t.Fatalf("saved.AgentSetup[claude-code] = %q; want %q", saved.AgentSetup["claude-code"], "yes")
	}
	if saved.TailscaleIP != "100.64.0.1" {
		t.Fatalf("saved.TailscaleIP = %q; want %q", saved.TailscaleIP, "100.64.0.1")
	}
	if saved.PodmanPath == "" {
		t.Fatal("saved.PodmanPath is empty; want a path")
	}
	_ = cfgDir
}

func TestSAGConfigMode0644(t *testing.T) {
	dir := t.TempDir()
	oldUmask := syscall.Umask(0o077)
	defer syscall.Umask(oldUmask)

	created, err := createSAGConfig(dir)
	if err != nil {
		t.Fatalf("createSAGConfig: %v", err)
	}
	if !created {
		t.Fatal("createSAGConfig: want created=true")
	}

	info, err := os.Stat(filepath.Join(dir, "sag.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o644 {
		t.Fatalf("sag.yaml mode = %04o; want 0644", info.Mode().Perm())
	}
}

func TestCheckHealthzClientTimeout(t *testing.T) {
	if healthzClientTimeout != 5*time.Second {
		t.Fatalf("healthzClientTimeout = %v; want 5s", healthzClientTimeout)
	}
}

func TestStep12RunsHealthzWhenRunningAndUnchanged(t *testing.T) {
	dir := t.TempDir()
	deps := baseDeps(dir)

	healthzCalled := false
	startCalled := false
	deps.ServiceRunning = func(_ context.Context, _ service.Config) (bool, error) { return true, nil }
	deps.CheckHealthz = func(_ context.Context, _ string) error {
		healthzCalled = true
		return nil
	}
	deps.ServiceStart = func(_ context.Context, _ service.Config, _ stepline.Writer) error {
		startCalled = true
		return nil
	}

	if err := Run(context.Background(), Flags{Autostart: "yes"}, deps); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !healthzCalled {
		t.Fatal("CheckHealthz not called when running=true and no changes")
	}
	if startCalled {
		t.Fatal("ServiceStart called when running=true and no changes")
	}
}

func TestStep12RunsStartWhenChanged(t *testing.T) {
	dir := t.TempDir()
	deps := baseDeps(dir)

	deps.ServiceRunning = func(_ context.Context, _ service.Config) (bool, error) { return true, nil }
	deps.MasterKey = func(_ context.Context, _ podman.Command, _ stepline.Writer) (bool, error) {
		return true, nil
	}
	healthzCalled := false
	startCalled := false
	deps.CheckHealthz = func(_ context.Context, _ string) error {
		healthzCalled = true
		return nil
	}
	deps.ServiceStart = func(_ context.Context, _ service.Config, _ stepline.Writer) error {
		startCalled = true
		return nil
	}

	if err := Run(context.Background(), Flags{Autostart: "yes"}, deps); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !startCalled {
		t.Fatal("ServiceStart not called when changed=true")
	}
	if healthzCalled {
		t.Fatal("CheckHealthz called when changed=true")
	}
}

func TestUID0Refused(t *testing.T) {
	deps := Deps{
		GOOS:   func() string { return "darwin" },
		GOARCH: func() string { return "arm64" },
		OpenFile: func(path string) error {
			return os.ErrNotExist
		},
		UID:     func() string { return "0" },
		Home:    func() string { return "/root" },
		SAGPath: func() (string, error) { return "/usr/local/bin/sag", nil },
		Out:     stepline.New(io.Discard),
		Print:   io.Discard,
	}
	steps := deps.BuildSteps()
	s := &State{Out: deps.Out}
	err := steps[0].Run(context.Background(), s)
	if err == nil {
		t.Fatal("step platform-check: want error for UID 0, got nil")
	}
	if err.Error() != "bootstrap: refused to run as UID 0" {
		t.Fatalf("error = %q; want UID 0 refusal message", err.Error())
	}
}

func TestContainerRefused(t *testing.T) {
	deps := Deps{
		GOOS:   func() string { return "darwin" },
		GOARCH: func() string { return "arm64" },
		OpenFile: func(path string) error {
			if path == "/run/.containerenv" {
				return nil
			}
			return os.ErrNotExist
		},
		UID:     func() string { return "501" },
		Home:    func() string { return "/home/user" },
		SAGPath: func() (string, error) { return "/usr/local/bin/sag", nil },
		Out:     stepline.New(io.Discard),
		Print:   io.Discard,
	}
	steps := deps.BuildSteps()
	s := &State{Out: deps.Out}
	err := steps[0].Run(context.Background(), s)
	if err == nil {
		t.Fatal("step platform-check: want error inside container, got nil")
	}
	if err.Error() != "bootstrap: refused to run inside a container" {
		t.Fatalf("error = %q; want container refusal message", err.Error())
	}
}
