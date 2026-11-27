package credential

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/kanthorlabs/sag/internal/config"
	"github.com/kanthorlabs/sag/internal/podman"
	"github.com/kanthorlabs/sag/internal/prompt"
	"github.com/kanthorlabs/sag/internal/secret"
	"github.com/kanthorlabs/sag/internal/testhook"
)

var (
	ErrDeleteFlagRejected = errors.New("--yes, --stdin, and --json are not accepted")
	ErrDeleteWrongName    = errors.New("typed name does not match; no change")
)

type DeleteOptions struct {
	CfgDir       string
	Backend      secret.SecureStorage
	Podman       podman.Command
	ServiceStart func(ctx context.Context, out io.Writer) error
	Out          io.Writer
	Err          io.Writer
	Image        string
}

func ParseDeleteArgs(args []string) (string, error) {
	fset := flag.NewFlagSet("credential delete", flag.ContinueOnError)
	fset.SetOutput(io.Discard)
	var yes, stdin, jsonOut bool
	fset.BoolVar(&yes, "yes", false, "")
	fset.BoolVar(&stdin, "stdin", false, "")
	fset.BoolVar(&jsonOut, "json", false, "")

	if err := fset.Parse(args); err != nil {
		return "", ErrDeleteFlagRejected
	}
	if yes || stdin || jsonOut {
		return "", ErrDeleteFlagRejected
	}
	if fset.NArg() != 1 {
		return "", fmt.Errorf("expected exactly one argument <name>")
	}
	return fset.Arg(0), nil
}

type deleteInfo struct {
	hasEntry bool
	kind     string
	agents   []string
	owned    []string
	unowned  []string
}

func collectDeleteState(ctx context.Context, name, cfgDir string, backend secret.SecureStorage) (deleteInfo, error) {
	var info deleteInfo

	cfg, err := loadSAGConfig(cfgDir)
	if err != nil {
		return info, err
	}

	res, hasEntry := cfg.Resources[name]
	info.hasEntry = hasEntry
	var referenced []string
	if hasEntry {
		info.kind = res.Kind
		info.agents = res.Agents
		if res.Connection != "" {
			referenced = append(referenced, string(res.Connection))
		}
		if res.TLSCA != "" {
			referenced = append(referenced, string(res.TLSCA))
		}
	}

	metas, err := backend.List(ctx, map[string]string{"sag.resource": name})
	if err != nil {
		if errors.Is(err, secret.ErrUnavailable) {
			return info, fmt.Errorf("backend podman unreachable; no change")
		}
		return info, err
	}

	ownedSet := make(map[string]bool, len(metas))
	for _, m := range metas {
		info.owned = append(info.owned, m.Name)
		ownedSet[m.Name] = true
	}

	for _, ref := range referenced {
		if !ownedSet[ref] {
			info.unowned = append(info.unowned, ref)
		}
	}

	if !hasEntry && len(info.owned) == 0 {
		return info, fmt.Errorf("resource %s not found", name)
	}

	return info, nil
}

func printDeleteInfo(tty *os.File, name string, info deleteInfo) {
	fmt.Fprintf(tty, "resource: %s\n", name)
	if info.hasEntry {
		fmt.Fprintf(tty, "kind: %s\n", info.kind)
		fmt.Fprintf(tty, "agents: %s\n", strings.Join(info.agents, ", "))
	}
	for _, s := range info.owned {
		fmt.Fprintf(tty, "secret: %s\n", s)
	}
}

func readConfirmLine(tty *os.File) (string, error) {
	r := bufio.NewReader(tty)
	line, err := r.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	line = strings.TrimSuffix(line, "\n")
	line = strings.TrimSuffix(line, "\r")
	return line, nil
}

func isContainerRunning(ctx context.Context, pm podman.Command) bool {
	out, err := pm.Run(ctx, nil, "container", "inspect", "sag", "--format", "{{.State.Running}}")
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) == "true"
}

func tolerateNoApproval(err error) error {
	if err == nil {
		return nil
	}
	var re *podman.RunError
	if errors.As(err, &re) && strings.Contains(re.Stderr, "approval: no approval record") {
		return nil
	}
	return err
}

func removeResourceFromYAML(sagYaml, name string) error {
	data, err := os.ReadFile(sagYaml)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("read sag.yaml: %w", err)
	}

	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return fmt.Errorf("parse sag.yaml: %w", err)
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
		return nil
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return nil
	}

	if !deleteFromMapping(root, "resources", name) {
		return nil
	}

	out, err := yaml.Marshal(&doc)
	if err != nil {
		return fmt.Errorf("marshal sag.yaml: %w", err)
	}

	if _, err := config.Parse(out); err != nil {
		return fmt.Errorf("validate sag.yaml: %w", err)
	}

	tmp := sagYaml + ".new"
	if err := os.WriteFile(tmp, out, 0o644); err != nil {
		return fmt.Errorf("write sag.yaml.new: %w", err)
	}
	if err := os.Rename(tmp, sagYaml); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename sag.yaml.new: %w", err)
	}
	return nil
}

func deleteFromMapping(root *yaml.Node, mapKey, entryKey string) bool {
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value != mapKey {
			continue
		}
		m := root.Content[i+1]
		if m.Kind != yaml.MappingNode {
			return false
		}
		for j := 0; j+1 < len(m.Content); j += 2 {
			if m.Content[j].Value == entryKey {
				m.Content = append(m.Content[:j], m.Content[j+2:]...)
				return true
			}
		}
		return false
	}
	return false
}

func Delete(ctx context.Context, name string, tty *os.File, opts DeleteOptions) error {
	if tty == nil {
		return fmt.Errorf("open /dev/tty: %w", prompt.ErrNoTerminal)
	}

	sagYaml := filepath.Join(opts.CfgDir, "sag.yaml")

	info, err := collectDeleteState(ctx, name, opts.CfgDir, opts.Backend)
	if err != nil {
		return err
	}

	printDeleteInfo(tty, name, info)

	fmt.Fprintf(tty, "type the resource name to confirm deletion: ")
	typed, err := readConfirmLine(tty)
	if err != nil {
		return fmt.Errorf("read confirmation: %w", err)
	}
	if typed != name {
		return ErrDeleteWrongName
	}

	wasRunning := isContainerRunning(ctx, opts.Podman)

	if wasRunning {
		revokeErr := tolerateNoApproval(revokeWithExec(ctx, name, opts.Podman))
		if revokeErr != nil {
			return fmt.Errorf("revoke approval: %w", revokeErr)
		}
	} else {
		revokeErr := tolerateNoApproval(revokeOneShot(ctx, name, opts.Podman, opts.Image))
		if revokeErr != nil {
			return fmt.Errorf("revoke approval (one-shot): %w", revokeErr)
		}
	}

	if err := removeResourceFromYAML(sagYaml, name); err != nil {
		return err
	}

	if wasRunning && opts.ServiceStart != nil {
		if err := opts.ServiceStart(ctx, opts.Out); err != nil {
			return fmt.Errorf("service start: %w", err)
		}
	}

	if testhook.FailCredDelete() {
		return errors.New("test hook: fail at step 8 (delete secrets)")
	}

	for _, s := range info.owned {
		if err := opts.Backend.Delete(ctx, s); err != nil {
			return fmt.Errorf("delete secret %s: %w", s, err)
		}
	}

	for _, s := range info.unowned {
		fmt.Fprintf(opts.Out, "skip %s not created by sag; not removed\n", s)
	}

	return nil
}

func revokeWithExec(ctx context.Context, name string, pm podman.Command) error {
	_, err := pm.Run(ctx, nil, "exec", "sag", "sag", "extension", "revoke", name)
	return err
}
