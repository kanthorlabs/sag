package bootstrap

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

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
	deps := Deps{Out: stepline.New(io.Discard), Print: io.Discard}
	built := deps.BuildSteps()
	if len(built) != len(Steps) {
		t.Fatalf("BuildSteps() len = %d; want %d", len(built), len(Steps))
	}
	for i, s := range Steps {
		if built[i].Name != s.Name {
			t.Fatalf("BuildSteps()[%d].Name = %q; want %q", i, built[i].Name, s.Name)
		}
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

	promptCalls := 0
	deps := baseDeps(dir)
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
}
