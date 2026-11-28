package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/kanthorlabs/sag/internal/config"
	"github.com/kanthorlabs/sag/internal/core"
	"github.com/kanthorlabs/sag/internal/credential"
	"github.com/kanthorlabs/sag/internal/hoststate"
	sagimage "github.com/kanthorlabs/sag/internal/image"
	"github.com/kanthorlabs/sag/internal/podman"
	"github.com/kanthorlabs/sag/internal/prompt"
	"github.com/kanthorlabs/sag/internal/secret"
	secretpodman "github.com/kanthorlabs/sag/internal/secret/podman"
	"github.com/kanthorlabs/sag/internal/service"
	"github.com/kanthorlabs/sag/internal/stepline"
)

const backendFlagPodman = "podman"

const credentialCreateUsage = "usage: sag credential create <name> --kind <kind> [--backend podman|aws-sm] [--agent <sub>]... [--description <text>] [--tls disable|verify-full|insecure-skip-verify] [--tls-ca <path>] [--stdin] [--accept-failed-check]"

var (
	runCredentialList    = credential.List
	runCredentialInspect = credential.Inspect
	buildCfgNoLockFn     = defaultBuildCfgNoLock
	openTTY              = func() (*os.File, error) {
		return os.OpenFile("/dev/tty", os.O_RDWR, 0)
	}
)

func defaultBuildCfgNoLock() (service.Config, func(), error) {
	p, err := hoststate.ResolvePaths()
	if err != nil {
		return service.Config{}, noop, err
	}
	answers, err := hoststate.LoadAnswers(p)
	if err != nil {
		return service.Config{}, noop, err
	}
	podmanPath, err := hoststate.FindPodman(answers.PodmanPath)
	if err != nil {
		return service.Config{}, noop, err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return service.Config{}, noop, err
	}
	binPath, err := os.Executable()
	if err != nil {
		return service.Config{}, noop, err
	}
	imageRef, _ := sagimage.Reference()
	return service.Config{
		Podman:  podman.Runner{Path: podmanPath},
		CfgDir:  p.Cfg,
		UID:     strconv.Itoa(os.Getuid()),
		Home:    home,
		BinPath: binPath,
		Image:   imageRef,
	}, noop, nil
}

func loadSAGConfig(cfgDir string) (*config.Config, error) {
	path := filepath.Join(cfgDir, "sag.yaml")
	cfg, err := config.Load(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &config.Config{Resources: map[string]config.Resource{}}, nil
		}
		return nil, err
	}
	return cfg, nil
}

func loadAWSBackend(cfgDir string) (secret.SecureStorage, error) {
	sagCfg, err := loadSAGConfig(cfgDir)
	if err != nil {
		return nil, err
	}
	if sagCfg.Storage.AWS.Region == "" {
		return nil, nil
	}
	return credential.BackendFor(sagCfg, secret.BackendAWSSM)
}

func credentialCreate(args []string, std IO) int {
	opts, ok := parseCredentialCreateFlags(args, std)
	if !ok {
		return ExitUsage
	}

	cfg, release, err := buildCfgFn()
	if err != nil {
		return fail(std, err)
	}
	defer release()

	runner, ok := cfg.Podman.(podman.Runner)
	if !ok {
		return fail(std, errors.New("internal: unexpected podman command type"))
	}

	var storage secret.SecureStorage
	if opts.Backend == secret.BackendAWSSM {
		sagCfg, err := loadSAGConfig(cfg.CfgDir)
		if err != nil {
			return fail(std, err)
		}
		storage, opts.StorageErr = credential.BackendFor(sagCfg, secret.BackendAWSSM)
	} else {
		opts.Backend = secret.BackendPodman
		storage = secretpodman.New(runner)
	}

	opts.Storage = storage
	opts.CfgDir = cfg.CfgDir
	opts.KindLookup = func(k string) (core.CredentialKind, bool) {
		a, ok := core.Lookup(k)
		if !ok {
			return nil, false
		}
		ck, ok := a.(core.CredentialKind)
		return ck, ok
	}
	opts.CheckCred = func(ctx context.Context, kind string, tls core.TLSMode, cred secret.Credential, caPem []byte) error {
		payload, err := buildCheckPayload(cred, tls, caPem)
		if err != nil {
			return err
		}
		_, err = runner.Run(ctx, strings.NewReader(string(payload)), "exec", "-i", "sag", "sag", "credential", "check", "--kind", kind, "--tls", string(tls))
		if err != nil {
			var re *podman.RunError
			if errors.As(err, &re) && re.Stderr != "" {
				return errors.New(re.Stderr)
			}
			return err
		}
		return nil
	}
	opts.ContainerRun = func(ctx context.Context) (bool, error) {
		out, err := runner.Run(ctx, nil, "container", "inspect", "--format", "{{.State.Running}}", "sag")
		if err != nil {
			return false, nil
		}
		return strings.TrimSpace(string(out)) == "true", nil
	}
	opts.ServiceStart = func(ctx context.Context, out io.Writer) error {
		return startFn(ctx, cfg, stepline.New(out))
	}
	opts.OpenTTY = openTTY
	opts.StdinReader = std.In
	opts.IsStdinTerm = std.IsTerminal(std.In)
	opts.Out = std.Out
	opts.Err = std.Err

	err = credential.Create(context.Background(), opts)
	if err == nil {
		return ExitOK
	}
	if errors.Is(err, prompt.ErrNoTerminal) {
		fmt.Fprintf(std.Err, "sag: %v\n", err)
		return ExitRefusal
	}
	return fail(std, err)
}

func parseCredentialCreateFlags(args []string, std IO) (credential.CreateOptions, bool) {
	if len(args) == 0 {
		fmt.Fprintln(std.Err, credentialCreateUsage)
		return credential.CreateOptions{}, false
	}
	name := args[0]
	args = args[1:]

	fs := flag.NewFlagSet("credential create", flag.ContinueOnError)
	fs.SetOutput(std.Err)

	var kind string
	fs.StringVar(&kind, "kind", "", "credential kind")

	var backend string
	fs.StringVar(&backend, "backend", backendFlagPodman, "secret backend: podman or aws-sm")

	var agents multiFlag
	fs.Var(&agents, "agent", "agent sub (repeatable)")

	var description string
	fs.StringVar(&description, "description", "", "human-readable description")

	var tls string
	fs.StringVar(&tls, "tls", string(core.TLSVerifyFull), "TLS mode: disable|verify-full|insecure-skip-verify")

	var tlsCAPath string
	fs.StringVar(&tlsCAPath, "tls-ca", "", "path to PEM CA certificate")

	var stdin bool
	fs.BoolVar(&stdin, "stdin", false, "read credential value from standard input")

	var acceptFailedCheck bool
	fs.BoolVar(&acceptFailedCheck, "accept-failed-check", false, "continue even if credential check fails")

	if err := fs.Parse(args); err != nil {
		return credential.CreateOptions{}, false
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(std.Err, credentialCreateUsage)
		return credential.CreateOptions{}, false
	}
	if kind == "" {
		fmt.Fprintln(std.Err, "sag: credential create: --kind is required")
		fmt.Fprintln(std.Err, credentialCreateUsage)
		return credential.CreateOptions{}, false
	}
	switch backend {
	case backendFlagPodman:
		backend = secret.BackendPodman
	case secret.BackendAWSSM:
	default:
		fmt.Fprintf(std.Err, "sag: credential create: --backend %q is not valid; use %s or %s\n", backend, backendFlagPodman, secret.BackendAWSSM)
		return credential.CreateOptions{}, false
	}

	var tlsMode core.TLSMode
	switch tls {
	case string(core.TLSDisable):
		tlsMode = core.TLSDisable
	case string(core.TLSVerifyFull):
		tlsMode = core.TLSVerifyFull
	case string(core.TLSInsecureSkipVerify):
		tlsMode = core.TLSInsecureSkipVerify
	default:
		fmt.Fprintf(std.Err, "sag: credential create: --tls %q is not valid; use disable, verify-full or insecure-skip-verify\n", tls)
		return credential.CreateOptions{}, false
	}

	var tlsCAPem []byte
	if tlsCAPath != "" {
		data, err := os.ReadFile(tlsCAPath)
		if err != nil {
			fmt.Fprintf(std.Err, "sag: credential create: --tls-ca: %v\n", err)
			return credential.CreateOptions{}, false
		}
		tlsCAPem = data
	}

	return credential.CreateOptions{
		Name:              name,
		Kind:              kind,
		Backend:           backend,
		Agents:            []string(agents),
		Description:       description,
		TLS:               tlsMode,
		TLSCAPem:          tlsCAPem,
		Stdin:             stdin,
		AcceptFailedCheck: acceptFailedCheck,
	}, true
}

type checkPayload struct {
	Value string  `json:"value"`
	TLSCA *string `json:"tls_ca"`
}

func buildCheckPayload(cred secret.Credential, _ core.TLSMode, caPem []byte) ([]byte, error) {
	p := checkPayload{
		Value: string(cred.Reveal()),
	}
	if len(caPem) > 0 {
		s := string(caPem)
		p.TLSCA = &s
	}
	return json.Marshal(p)
}

func credentialList(args []string, std IO) int {
	fs := flag.NewFlagSet("credential list", flag.ContinueOnError)
	fs.SetOutput(std.Err)
	jsonOut := fs.Bool("json", false, "output JSON")
	if err := fs.Parse(args); err != nil {
		return ExitUsage
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(std.Err, "usage: sag credential list [--json]")
		return ExitUsage
	}

	cfg, release, err := buildCfgNoLockFn()
	if err != nil {
		return fail(std, err)
	}
	defer release()

	runner, ok := cfg.Podman.(podman.Runner)
	if !ok {
		return fail(std, errors.New("internal: unexpected podman command type"))
	}

	awsBackend, err := loadAWSBackend(cfg.CfgDir)
	if err != nil {
		return fail(std, err)
	}

	opts := credential.ListOptions{
		SAGYaml:    filepath.Join(cfg.CfgDir, "sag.yaml"),
		CAPem:      filepath.Join(cfg.CfgDir, "ca.pem"),
		Backend:    secretpodman.New(runner),
		AWSBackend: awsBackend,
		Podman:     runner,
		Stderr:     std.Err,
	}

	entries, err := runCredentialList(context.Background(), opts)
	if err != nil {
		return fail(std, err)
	}

	if *jsonOut {
		out, err := json.Marshal(map[string]any{"credentials": entries})
		if err != nil {
			return fail(std, err)
		}
		fmt.Fprintln(std.Out, string(out))
		return ExitOK
	}

	for i, e := range entries {
		if i > 0 {
			fmt.Fprintln(std.Out)
		}
		printEntryText(std.Out, e, false)
	}
	return ExitOK
}

func credentialInspect(args []string, std IO) int {
	if len(args) == 0 {
		fmt.Fprintln(std.Err, "usage: sag credential inspect <name> [--json]")
		return ExitUsage
	}
	name := args[0]
	args = args[1:]

	fs := flag.NewFlagSet("credential inspect", flag.ContinueOnError)
	fs.SetOutput(std.Err)
	jsonOut := fs.Bool("json", false, "output JSON")
	if err := fs.Parse(args); err != nil {
		return ExitUsage
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(std.Err, "usage: sag credential inspect <name> [--json]")
		return ExitUsage
	}

	cfg, release, err := buildCfgNoLockFn()
	if err != nil {
		return fail(std, err)
	}
	defer release()

	runner, ok := cfg.Podman.(podman.Runner)
	if !ok {
		return fail(std, errors.New("internal: unexpected podman command type"))
	}

	awsBackend, err := loadAWSBackend(cfg.CfgDir)
	if err != nil {
		return fail(std, err)
	}

	opts := credential.InspectOptions{
		SAGYaml:    filepath.Join(cfg.CfgDir, "sag.yaml"),
		Backend:    secretpodman.New(runner),
		AWSBackend: awsBackend,
		Podman:     runner,
		Stderr:     std.Err,
	}

	entry, err := runCredentialInspect(context.Background(), name, opts)
	if err != nil {
		if errors.Is(err, credential.ErrResourceNotFound) {
			fmt.Fprintf(std.Err, "sag: resource %s not found\n", name)
			return ExitFailure
		}
		return fail(std, err)
	}

	if *jsonOut {
		out, err := json.Marshal(entry)
		if err != nil {
			return fail(std, err)
		}
		fmt.Fprintln(std.Out, string(out))
		return ExitOK
	}

	printEntryText(std.Out, entry, true)
	return ExitOK
}

func printEntryText(w io.Writer, e credential.Entry, inspect bool) {
	fmt.Fprintf(w, "name: %s\n", e.Name)
	fmt.Fprintf(w, "kind: %s\n", e.Kind)
	fmt.Fprintf(w, "description: %s\n", e.Description)
	fmt.Fprintf(w, "tls: %s\n", e.TLS)
	fmt.Fprintf(w, "agents: %s\n", strings.Join(e.Agents, ","))
	for i, s := range e.Secrets {
		prefix := fmt.Sprintf("secrets[%d]", i)
		fmt.Fprintf(w, "%s.name: %s\n", prefix, s.Name)
		fmt.Fprintf(w, "%s.backend: %s\n", prefix, s.Backend)
		fmt.Fprintf(w, "%s.role: %s\n", prefix, s.Role)
		if s.Exists == nil {
			fmt.Fprintf(w, "%s.exists: null\n", prefix)
		} else {
			fmt.Fprintf(w, "%s.exists: %v\n", prefix, *s.Exists)
		}
		fmt.Fprintf(w, "%s.owned: %v\n", prefix, s.Owned)
		if s.CreatedAt == nil {
			fmt.Fprintf(w, "%s.created_at: null\n", prefix)
		} else {
			fmt.Fprintf(w, "%s.created_at: %s\n", prefix, s.CreatedAt.UTC().Format(time.RFC3339))
		}
		if s.Mounted == nil {
			fmt.Fprintf(w, "%s.mounted: null\n", prefix)
		} else {
			fmt.Fprintf(w, "%s.mounted: %v\n", prefix, *s.Mounted)
		}
	}
	if inspect {
		if e.ContainerRunning == nil {
			fmt.Fprintf(w, "container_running: null\n")
		} else {
			fmt.Fprintf(w, "container_running: %v\n", *e.ContainerRunning)
		}
	}
}

func credentialDelete(args []string, std IO) int {
	name, err := credential.ParseDeleteArgs(args)
	if err != nil {
		fmt.Fprintf(std.Err, "sag: %v\n", err)
		return ExitUsage
	}

	tty, err := openTTY()
	if err != nil {
		fmt.Fprintf(std.Err, "sag: open /dev/tty: %v\n", err)
		return ExitRefusal
	}
	defer tty.Close()

	cfg, release, err := buildCfgFn()
	if err != nil {
		return fail(std, err)
	}
	defer release()

	runner, ok := cfg.Podman.(podman.Runner)
	if !ok {
		return fail(std, errors.New("internal: unexpected podman command type"))
	}

	awsBackend, err := loadAWSBackend(cfg.CfgDir)
	if err != nil {
		return fail(std, err)
	}

	opts := credential.DeleteOptions{
		CfgDir:     cfg.CfgDir,
		Backend:    secretpodman.New(runner),
		AWSBackend: awsBackend,
		Podman:     cfg.Podman,
		ServiceStart: func(ctx context.Context, out io.Writer) error {
			return startFn(ctx, cfg, stepline.New(out))
		},
		Out:   std.Out,
		Err:   std.Err,
		Image: cfg.Image,
	}

	err = credential.Delete(context.Background(), name, tty, opts)
	if err == nil {
		return ExitOK
	}
	if errors.Is(err, credential.ErrDeleteWrongName) {
		fmt.Fprintf(std.Err, "sag: %v\n", err)
		return ExitRefusal
	}
	if errors.Is(err, prompt.ErrNoTerminal) {
		fmt.Fprintf(std.Err, "sag: %v\n", err)
		return ExitRefusal
	}
	return fail(std, err)
}
