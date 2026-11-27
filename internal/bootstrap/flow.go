package bootstrap

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/kanthorlabs/sag/internal/agentsetup"
	"github.com/kanthorlabs/sag/internal/endpointcert"
	"github.com/kanthorlabs/sag/internal/hoststate"
	"github.com/kanthorlabs/sag/internal/image"
	"github.com/kanthorlabs/sag/internal/masterkey"
	"github.com/kanthorlabs/sag/internal/podman"
	"github.com/kanthorlabs/sag/internal/portcheck"
	"github.com/kanthorlabs/sag/internal/prompt"
	"github.com/kanthorlabs/sag/internal/service"
	"github.com/kanthorlabs/sag/internal/stepline"
	"github.com/kanthorlabs/sag/internal/tailscale"
)

type Deps struct {
	GOOS     func() string
	GOARCH   func() string
	OpenFile func(string) error
	UID      func() string
	Home     func() string
	SAGPath  func() (string, error)

	ResolvePaths func() (hoststate.Paths, error)
	EnsureCfg    func(hoststate.Paths) (bool, error)
	AcquireLock  func(hoststate.Paths) (*hoststate.Lock, error)
	ReleaseLock  func(*hoststate.Lock) error
	LoadAnswers  func(hoststate.Paths) (hoststate.Answers, error)
	SaveAnswers  func(hoststate.Paths, hoststate.Answers) error
	FindPodman   func(stored string) (string, error)

	PlanDeps      func(context.Context, *State) error
	PlanTailscale func(context.Context, *State) error
	PlanPort      func(context.Context, *State) error

	ResolveYesNo func(prompt.Asker, string, string, string, string) (string, error)
	Asker        prompt.Asker

	PodmanCmd      func(path string) podman.Command
	ImageRef       func() string
	ImageEnsure    func(context.Context, podman.Command, string, stepline.Writer) (bool, error)
	MasterKey      func(context.Context, podman.Command, stepline.Writer) (bool, error)
	CertEnsure     func(context.Context, podman.Command, endpointcert.Config, time.Time, stepline.Writer) (bool, error)
	SAGYAMLCreate  func(cfgDir string) (bool, error)
	ApplyAutostart func(context.Context, service.Config, bool, stepline.Writer) (bool, error)
	ServiceRunning func(context.Context, service.Config) (bool, error)
	ServiceStart   func(context.Context, service.Config, stepline.Writer) error
	CheckHealthz   func(ctx context.Context, cfgDir string) error

	Environ       func() map[string]string
	HarnessInPATH func(agentsetup.Harness, map[string]string) bool
	PlanOnly      func(context.Context, agentsetup.Options) (agentsetup.Plan, error)
	AgentRun      func(context.Context, agentsetup.Options) error

	Now   func() time.Time
	Out   stepline.Writer
	Print io.Writer
}

func DefaultDeps(asker prompt.Asker, out stepline.Writer, print io.Writer) Deps {
	return Deps{
		GOOS:   func() string { return runtime.GOOS },
		GOARCH: func() string { return runtime.GOARCH },
		OpenFile: func(path string) error {
			f, err := os.Open(path)
			if err != nil {
				return err
			}
			f.Close()
			return nil
		},
		UID:  func() string { return fmt.Sprintf("%d", os.Getuid()) },
		Home: func() string { return os.Getenv("HOME") },
		SAGPath: func() (string, error) {
			return os.Executable()
		},

		ResolvePaths: hoststate.ResolvePaths,
		EnsureCfg:    hoststate.EnsureCfg,
		AcquireLock:  hoststate.AcquireLock,
		ReleaseLock:  func(l *hoststate.Lock) error { return l.Release() },
		LoadAnswers:  hoststate.LoadAnswers,
		SaveAnswers:  hoststate.SaveAnswers,
		FindPodman:   hoststate.FindPodman,

		PlanDeps:      defaultPlanDeps(hoststate.FindPodman),
		PlanTailscale: defaultPlanTailscale(tailscale.LookupCommand, tailscale.Detect),
		PlanPort:      defaultPlanPort(portcheck.Check),

		ResolveYesNo: prompt.ResolveYesNo,
		Asker:        asker,

		PodmanCmd: func(path string) podman.Command { return podman.Runner{Path: path} },
		ImageRef:  func() string { ref, _ := image.Reference(); return ref },
		ImageEnsure: func(ctx context.Context, pm podman.Command, ref string, w stepline.Writer) (bool, error) {
			return image.Ensure(ctx, pm, ref, w)
		},
		MasterKey: func(ctx context.Context, pm podman.Command, w stepline.Writer) (bool, error) {
			return masterkey.Ensure(ctx, pm, w)
		},
		CertEnsure: func(ctx context.Context, pm podman.Command, cfg endpointcert.Config, now time.Time, w stepline.Writer) (bool, error) {
			return endpointcert.Ensure(ctx, pm, cfg, now, w)
		},
		SAGYAMLCreate:  createSAGConfig,
		ApplyAutostart: service.ApplyAutostart,
		ServiceRunning: service.Running,
		ServiceStart:   service.Start,
		CheckHealthz:   defaultCheckHealthz,

		Environ:       defaultEnvMap,
		HarnessInPATH: defaultHarnessInPATH,
		PlanOnly:      agentsetup.PlanOnly,
		AgentRun: func(ctx context.Context, opts agentsetup.Options) error {
			return convertAgentRunError(agentsetup.Run(ctx, opts))
		},

		Now:   time.Now,
		Out:   out,
		Print: print,
	}
}

const healthzClientTimeout = 5 * time.Second

func newHealthzClient(caData []byte) (*http.Client, error) {
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caData) {
		return nil, fmt.Errorf("bootstrap: no valid CA certificate")
	}
	return &http.Client{
		Timeout: healthzClientTimeout,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{RootCAs: pool},
		},
	}, nil
}

func defaultCheckHealthz(ctx context.Context, cfgDir string) error {
	caPath := filepath.Join(cfgDir, "ca.pem")
	caData, err := os.ReadFile(caPath)
	if err != nil {
		return err
	}
	client, err := newHealthzClient(caData)
	if err != nil {
		return fmt.Errorf("bootstrap: no valid CA certificate in %s", caPath)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://localhost:8443/healthz", nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bootstrap: healthz returned %d", resp.StatusCode)
	}
	return nil
}

func (d Deps) BuildSteps() []Step {
	var as agentRunState
	return []Step{
		{Name: "platform-check", Run: d.stepPlatformCheck},
		{Name: "select-pins", Run: d.stepSelectPins},
		{Name: "plan", Run: d.stepPlan},
		{Name: "lock", Run: d.stepLock},
		{Name: "plan-again", Run: d.stepPlanAgain},
		{Name: "deps", Run: d.stepDeps},
		{Name: "image", Run: d.stepImage},
		{Name: "master-key", Run: d.stepMasterKey},
		{Name: "cert", Run: d.stepCert},
		{Name: "sag-yaml", Run: d.stepSAGYAML},
		{Name: "autostart", Run: d.stepAutostart},
		{Name: "service-start", Run: d.stepServiceStart},
		{Name: "tailscale", Run: d.stepTailscale},
		{Name: "agent-setup", Run: func(ctx context.Context, s *State) error {
			return d.runAgentSetup(ctx, s, &as)
		}},
		{Name: "print-endpoint", Run: func(ctx context.Context, s *State) error {
			return d.printEndpoint(ctx, s, &as)
		}},
	}
}

func Run(ctx context.Context, flags Flags, deps Deps) error {
	s := &State{
		Flags: flags,
		Asker: deps.Asker,
		Out:   deps.Out,
	}
	for _, step := range deps.BuildSteps() {
		if err := step.Run(ctx, s); err != nil {
			if s.Lock != nil && deps.ReleaseLock != nil {
				_ = deps.ReleaseLock(s.Lock)
			}
			return err
		}
	}
	if s.Lock != nil && deps.ReleaseLock != nil {
		_ = deps.ReleaseLock(s.Lock)
	}
	return nil
}

func (d Deps) stepPlatformCheck(_ context.Context, s *State) error {
	if err := checkPlatform(d.GOOS(), d.GOARCH()); err != nil {
		return err
	}
	uid := d.UID()
	if uid == "0" {
		return fmt.Errorf("bootstrap: refused to run as UID 0")
	}
	if err := d.OpenFile("/run/.containerenv"); err == nil {
		return fmt.Errorf("bootstrap: refused to run inside a container")
	}
	s.UID = uid
	s.Home = d.Home()
	sagPath, err := d.SAGPath()
	if err != nil {
		return err
	}
	s.SAGPath = sagPath
	return nil
}

func (d Deps) stepPlan(ctx context.Context, s *State) error {
	paths, err := d.ResolvePaths()
	if err != nil {
		return err
	}
	s.Paths = paths
	return d.planOnce(ctx, s)
}

func (d Deps) stepLock(_ context.Context, s *State) error {
	created, err := d.EnsureCfg(s.Paths)
	if err != nil {
		return err
	}
	s.CfgCreated = created
	lock, err := d.AcquireLock(s.Paths)
	if err != nil {
		return err
	}
	s.Lock = lock
	return nil
}

func (d Deps) stepPlanAgain(ctx context.Context, s *State) error {
	return d.planOnce(ctx, s)
}

func (d Deps) stepImage(ctx context.Context, s *State) error {
	pm := d.PodmanCmd(s.PodmanPath)
	_, err := d.ImageEnsure(ctx, pm, d.ImageRef(), s.Out)
	return err
}

func (d Deps) stepMasterKey(ctx context.Context, s *State) error {
	pm := d.PodmanCmd(s.PodmanPath)
	changed, err := d.MasterKey(ctx, pm, s.Out)
	if err != nil {
		return err
	}
	if changed {
		s.Changed = true
	}
	return nil
}

func (d Deps) stepCert(ctx context.Context, s *State) error {
	pm := d.PodmanCmd(s.PodmanPath)
	cfg := endpointcert.Config{
		CfgDir:      s.Paths.Cfg,
		TailscaleIP: s.TailscaleState.Address,
	}
	changed, err := d.CertEnsure(ctx, pm, cfg, d.Now(), s.Out)
	if err != nil {
		return err
	}
	if changed {
		s.Changed = true
	}
	return nil
}

func (d Deps) stepSAGYAML(_ context.Context, s *State) error {
	changed, err := d.SAGYAMLCreate(s.Paths.Cfg)
	if err != nil {
		return err
	}
	if changed {
		s.Changed = true
		s.Out.Line(stepline.StatusInstall, "sag-yaml", "")
		return nil
	}
	s.Out.Line(stepline.StatusOK, "sag-yaml", "")
	return nil
}

func (d Deps) stepAutostart(ctx context.Context, s *State) error {
	enable := s.Answers.Autostart == "yes"
	cfg := d.svcConfig(s)
	changed, err := d.ApplyAutostart(ctx, cfg, enable, s.Out)
	if err != nil {
		return err
	}
	if changed {
		s.Changed = true
	}
	return nil
}

func (d Deps) stepServiceStart(ctx context.Context, s *State) error {
	cfg := d.svcConfig(s)
	running, err := d.ServiceRunning(ctx, cfg)
	if err != nil {
		return err
	}
	if !running || s.Changed {
		return d.ServiceStart(ctx, cfg, s.Out)
	}
	if err := d.CheckHealthz(ctx, s.Paths.Cfg); err != nil {
		return err
	}
	s.Out.Line(stepline.StatusOK, "sag", "")
	return nil
}

func (d Deps) planOnce(ctx context.Context, s *State) error {
	fresh, err := d.LoadAnswers(s.Paths)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}

	s.TailscaleState.Address = fresh.TailscaleIP
	if s.Answers.PodmanPath == "" {
		s.Answers.PodmanPath = fresh.PodmanPath
	}

	if err := d.PlanDeps(ctx, s); err != nil {
		return err
	}
	if err := d.PlanTailscale(ctx, s); err != nil {
		return err
	}
	if err := d.PlanPort(ctx, s); err != nil {
		return err
	}

	storedAutostart := s.Answers.Autostart
	if storedAutostart == "" {
		storedAutostart = fresh.Autostart
	}
	autostart, err := d.ResolveYesNo(s.Asker, s.Flags.Autostart, storedAutostart, "Start SAG at login? [y/N] ", "--autostart")
	if err != nil {
		return err
	}
	s.Answers.Autostart = autostart

	environ := make(map[string]string)
	if d.Environ != nil {
		environ = d.Environ()
	}
	if err := d.planAgentSetup(ctx, s, agentSetupFresh(fresh.AgentSetup), environ, s.Home); err != nil {
		return err
	}

	if s.Lock != nil {
		merged := fresh
		merged.Autostart = s.Answers.Autostart
		merged.PodmanPath = s.PodmanPath
		if s.Answers.AgentSetup != nil {
			if merged.AgentSetup == nil {
				merged.AgentSetup = make(map[string]string)
			}
			for k, v := range s.Answers.AgentSetup {
				merged.AgentSetup[k] = v
			}
		}
		if err := d.SaveAnswers(s.Paths, merged); err != nil {
			return err
		}
		s.Answers = merged
	}

	return nil
}

func (d Deps) svcConfig(s *State) service.Config {
	return service.Config{
		Podman:  d.PodmanCmd(s.PodmanPath),
		CfgDir:  s.Paths.Cfg,
		UID:     s.UID,
		Home:    s.Home,
		BinPath: s.SAGPath,
		Image:   d.ImageRef(),
	}
}
