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
	"strings"

	"github.com/kanthorlabs/sag/internal/core"
	"github.com/kanthorlabs/sag/internal/credential"
	"github.com/kanthorlabs/sag/internal/podman"
	"github.com/kanthorlabs/sag/internal/prompt"
	"github.com/kanthorlabs/sag/internal/secret"
	secretpodman "github.com/kanthorlabs/sag/internal/secret/podman"
	"github.com/kanthorlabs/sag/internal/stepline"
)

const credentialCreateUsage = "usage: sag credential create <name> --kind <kind> [--agent <sub>]... [--description <text>] [--tls disable|verify-full|insecure-skip-verify] [--tls-ca <path>] [--stdin] [--accept-failed-check]"

var (
	runCredentialList   = credential.List
	runCredentialInspect = credential.Inspect
)

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
	storage := secretpodman.New(runner)

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
		return err
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
	opts.OpenTTY = func() (*os.File, error) {
		return os.OpenFile("/dev/tty", os.O_RDWR, 0)
	}
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

	var agents multiFlag
	fs.Var(&agents, "agent", "agent sub (repeatable)")

	var description string
	fs.StringVar(&description, "description", "", "human-readable description")

	var tls string
	fs.StringVar(&tls, "tls", string(core.TLSDisable), "TLS mode: disable|verify-full|insecure-skip-verify")

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

	cfg, release, err := buildCfgFn()
	if err != nil {
		return fail(std, err)
	}
	defer release()

	runner, ok := cfg.Podman.(podman.Runner)
	if !ok {
		return fail(std, errors.New("internal: unexpected podman command type"))
	}

	opts := credential.ListOptions{
		SAGYaml: filepath.Join(cfg.CfgDir, "sag.yaml"),
		Backend: secretpodman.New(runner),
		Podman:  runner,
		Stderr:  std.Err,
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

	for _, e := range entries {
		fmt.Fprintf(std.Out, "%s kind=%s tls=%s agents=%s\n", e.Name, e.Kind, e.TLS, strings.Join(e.Agents, ","))
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

	cfg, release, err := buildCfgFn()
	if err != nil {
		return fail(std, err)
	}
	defer release()

	runner, ok := cfg.Podman.(podman.Runner)
	if !ok {
		return fail(std, errors.New("internal: unexpected podman command type"))
	}

	opts := credential.InspectOptions{
		SAGYaml: filepath.Join(cfg.CfgDir, "sag.yaml"),
		Backend: secretpodman.New(runner),
		Podman:  runner,
		Stderr:  std.Err,
	}

	entry, err := runCredentialInspect(context.Background(), name, opts)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
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

	fmt.Fprintf(std.Out, "name: %s\nkind: %s\ntls: %s\nagents: %s\n", entry.Name, entry.Kind, entry.TLS, strings.Join(entry.Agents, ", "))
	return ExitOK
}

func credentialDelete(args []string, std IO) int {
	return fail(std, errNotImplemented)
}
