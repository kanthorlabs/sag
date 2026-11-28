package credential

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/kanthorlabs/sag/internal/core"
	"github.com/kanthorlabs/sag/internal/prompt"
	"github.com/kanthorlabs/sag/internal/secret"
	"github.com/kanthorlabs/sag/internal/testhook"
)

type CreateOptions struct {
	Name              string
	Kind              string
	Agents            []string
	Description       string
	TLS               core.TLSMode
	TLSCAPem          []byte
	Stdin             bool
	AcceptFailedCheck bool
	Backend           string

	CfgDir string

	Storage      secret.SecureStorage
	StorageErr   error
	KindLookup   func(string) (core.CredentialKind, bool)
	CheckCred    func(ctx context.Context, kind string, tls core.TLSMode, cred secret.Credential, caPem []byte) error
	ServiceStart func(ctx context.Context, out io.Writer) error
	ContainerRun func(ctx context.Context) (bool, error)
	OpenTTY      func() (*os.File, error)
	StdinReader  io.Reader
	IsStdinTerm  bool
	Out          io.Writer
	Err          io.Writer
}

func Create(ctx context.Context, opts CreateOptions) error {
	if err := validateName(opts.Name); err != nil {
		return err
	}
	if err := checkResourceAbsent(opts.CfgDir, opts.Name); err != nil {
		return err
	}
	if opts.StorageErr == nil {
		if err := checkSecretAbsent(ctx, opts.Storage, secretConnName(opts.Backend, opts.Name)); err != nil {
			return err
		}
		if err := checkSecretAbsent(ctx, opts.Storage, secretCAName(opts.Backend, opts.Name)); err != nil {
			return err
		}
	}

	ck, ok := opts.KindLookup(opts.Kind)
	if !ok {
		return fmt.Errorf("unknown kind %q", opts.Kind)
	}
	if err := validateAgents(opts.Agents); err != nil {
		return err
	}
	if err := validateDescription(opts.Description); err != nil {
		return err
	}
	if err := validateTLSCAPem(opts.TLSCAPem); err != nil {
		return err
	}
	if opts.StorageErr != nil {
		return opts.StorageErr
	}

	running, err := opts.ContainerRun(ctx)
	if err != nil {
		return err
	}
	if !running {
		return errors.New("container sag stopped; run sag service start")
	}

	cred, err := readValue(opts, ck.CredentialPrompt())
	if err != nil {
		return err
	}

	notes, err := ck.ValidateCredential(cred)
	if err != nil {
		return err
	}

	if notes.LoopbackHost {
		fmt.Fprintln(opts.Err, "warn dsn host is a loopback address; in the container it is the container; for a database on this machine use host.containers.internal; on Linux the database must listen on a non-loopback address")
	}

	checkErr := opts.CheckCred(ctx, opts.Kind, opts.TLS, cred, opts.TLSCAPem)
	if checkErr != nil {
		if opts.AcceptFailedCheck {
			fmt.Fprintf(opts.Err, "warn %v\n", checkErr)
		} else {
			return checkErr
		}
	}

	return applyCreate(ctx, opts, cred)
}

func applyCreate(ctx context.Context, opts CreateOptions, cred secret.Credential) error {
	connName := secretConnName(opts.Backend, opts.Name)
	caName := secretCAName(opts.Backend, opts.Name)

	snapshot, err := snapshotYAML(opts.CfgDir)
	if err != nil {
		return err
	}

	var created []string

	if err := opts.Storage.Put(ctx, connName, cred, map[string]string{
		"sag.resource": opts.Name,
		"sag.kind":     opts.Kind,
		"sag.role":     "connection",
	}); err != nil {
		return fmt.Errorf("put connection secret: %w", err)
	}
	created = append(created, connName)

	if testhook.FailCredCreate() {
		return rollback(ctx, opts, snapshot, created, errors.New("test hook: fail at step 3"))
	}

	if len(opts.TLSCAPem) > 0 {
		caCred := secret.NewCredential(caName, opts.TLSCAPem)
		if err := opts.Storage.Put(ctx, caName, caCred, map[string]string{
			"sag.resource": opts.Name,
			"sag.kind":     opts.Kind,
			"sag.role":     "tls_ca",
		}); err != nil {
			return rollback(ctx, opts, snapshot, created, fmt.Errorf("put CA secret: %w", err))
		}
		created = append(created, caName)
	}

	if testhook.FailTLSCACreate() {
		return rollback(ctx, opts, snapshot, created, errors.New("test hook: fail at step 3 (tls-ca)"))
	}

	connRef := backendRefPrefix(opts.Backend) + connName
	caRef := backendRefPrefix(opts.Backend) + caName
	if err := addResourceToYAML(opts.CfgDir, opts.Name, opts.Kind, opts.Description, opts.TLS, opts.Agents, connRef, caRef, len(opts.TLSCAPem) > 0); err != nil {
		return rollback(ctx, opts, snapshot, created, err)
	}

	if err := opts.ServiceStart(ctx, opts.Out); err != nil {
		return rollback(ctx, opts, snapshot, created, fmt.Errorf("service start: %w", err))
	}

	fmt.Fprintf(opts.Out, "resource %s created; SAG restarted; run sag resource test %s\n", opts.Name, opts.Name)
	return nil
}

func rollback(ctx context.Context, opts CreateOptions, snapshot []byte, created []string, cause error) error {
	if err := restoreYAML(opts.CfgDir, snapshot); err != nil {
		fmt.Fprintf(opts.Err, "warn rollback: restore YAML: %v\n", err)
	}
	for _, name := range created {
		if err := opts.Storage.Delete(ctx, name); err != nil {
			fmt.Fprintf(opts.Err, "warn rollback: delete %s: %v\n", name, err)
		}
	}
	if err := opts.ServiceStart(ctx, opts.Out); err != nil {
		fmt.Fprintf(opts.Err, "warn rollback: service start: %v\n", err)
	}
	return cause
}

func readValue(opts CreateOptions, credPrompt string) (secret.Credential, error) {
	if opts.Stdin {
		if opts.IsStdinTerm {
			return secret.Credential{}, fmt.Errorf("--stdin: standard input is a terminal: %w", prompt.ErrNoTerminal)
		}
		return readStdin(opts.StdinReader, false)
	}
	tty, err := opts.OpenTTY()
	if err != nil {
		return secret.Credential{}, fmt.Errorf("open /dev/tty: %w", prompt.ErrNoTerminal)
	}
	defer tty.Close()
	label := fmt.Sprintf("%s for %s (input hidden): ", credPrompt, opts.Name)
	return prompt.ReadHidden(tty, label)
}

func checkSecretAbsent(ctx context.Context, s secret.SecureStorage, name string) error {
	_, err := s.Metadata(ctx, name)
	if err == nil {
		return fmt.Errorf("secret %s exists; run sag credential delete %s", name, resourceNameFromSecret(name))
	}
	if errors.Is(err, secret.ErrNotFound) {
		return nil
	}
	return err
}

func secretConnName(backend, resourceName string) string {
	if backend == secret.BackendAWSSM {
		return "sag/" + resourceName + "/conn"
	}
	return "sag-res-" + resourceName + ".conn"
}

func secretCAName(backend, resourceName string) string {
	if backend == secret.BackendAWSSM {
		return "sag/" + resourceName + "/ca"
	}
	return "sag-res-" + resourceName + ".ca"
}

func backendRefPrefix(backend string) string {
	if backend == secret.BackendAWSSM {
		return secret.BackendAWSSM + ":"
	}
	return secret.BackendPodman + ":"
}

func resourceNameFromSecret(secretName string) string {
	s := strings.TrimPrefix(secretName, "sag-res-")
	if i := strings.LastIndex(s, "."); i >= 0 {
		return s[:i]
	}
	s = strings.TrimPrefix(s, "sag/")
	if i := strings.LastIndex(s, "/"); i >= 0 {
		return s[:i]
	}
	return s
}
