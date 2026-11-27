package dxh

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

const minFreeGiB = 10

type PreconditionConfig struct {
	CfgDir            string
	PortHeld          func() bool
	CfgExists         func() bool
	LaunchAgentExists func() bool
	Marker            bool
}

func CheckPreconditions(ctx context.Context, r Runner, cfg PreconditionConfig) error {
	if err := checkMachine(ctx, r); err != nil {
		return err
	}
	if err := CheckFreeSpace(ctx, r); err != nil {
		return err
	}
	if err := checkContainers(ctx, r); err != nil {
		return err
	}
	if cfg.PortHeld != nil && cfg.PortHeld() {
		return fmt.Errorf("precondition: port 8443 is held")
	}
	if err := checkVolumeAndImage(ctx, r); err != nil {
		return err
	}
	if err := checkSecrets(ctx, r); err != nil {
		return err
	}
	if cfg.CfgExists != nil && cfg.CfgExists() {
		return fmt.Errorf("precondition: cfg directory exists: %s", cfg.CfgDir)
	}
	if cfg.LaunchAgentExists != nil && cfg.LaunchAgentExists() {
		return fmt.Errorf("precondition: LaunchAgent plist exists")
	}
	return nil
}

func CheckFreeSpace(ctx context.Context, r Runner) error {
	out, err := r.Output(ctx, "podman", "machine", "ssh", "podman-machine-default", "df", "-Pk", "/var")
	if err != nil {
		return fmt.Errorf("precondition: df /var: %w", err)
	}
	gib, err := parseDFAvailable(string(out))
	if err != nil {
		return fmt.Errorf("precondition: df /var parse: %w", err)
	}
	if gib < minFreeGiB {
		return fmt.Errorf("precondition: fail podman machine free space %d GiB; need 10", gib)
	}
	return nil
}

func parseDFAvailable(output string) (int, error) {
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		if _, err := strconv.Atoi(fields[1]); err != nil {
			continue
		}
		avail, err := strconv.ParseInt(fields[3], 10, 64)
		if err != nil {
			continue
		}
		return int(avail / (1024 * 1024)), nil
	}
	return 0, fmt.Errorf("no data row in df output")
}

func checkMachine(ctx context.Context, r Runner) error {
	out, err := r.Output(ctx, "podman", "machine", "list", "--format", "{{.Name}} {{.Running}}")
	if err != nil {
		return fmt.Errorf("precondition: podman machine list: %w", err)
	}
	lines := parseLines(out)
	if len(lines) != 1 {
		return fmt.Errorf("precondition: expected exactly one podman machine, got %d", len(lines))
	}
	parts := strings.Fields(lines[0])
	if len(parts) < 2 {
		return fmt.Errorf("precondition: unexpected machine list format: %q", lines[0])
	}
	if parts[1] != "true" {
		return fmt.Errorf("precondition: podman machine %s is not running", parts[0])
	}
	return nil
}

func checkContainers(ctx context.Context, r Runner) error {
	for _, name := range []string{"sag", "sag-dev", "sag-dev-pg"} {
		out, err := r.Output(ctx, "podman", "ps", "-a", "--filter", "name=^"+name+"$", "--format", "{{.Names}}")
		if err != nil {
			return fmt.Errorf("precondition: podman ps %s: %w", name, err)
		}
		if strings.TrimSpace(string(out)) != "" {
			return fmt.Errorf("precondition: container %s exists", name)
		}
	}
	return nil
}

func checkVolumeAndImage(ctx context.Context, r Runner) error {
	_, err := r.Output(ctx, "podman", "volume", "inspect", "sag-state")
	if err == nil {
		return fmt.Errorf("precondition: volume sag-state exists")
	}
	out, err := r.Output(ctx, "podman", "image", "ls", "--filter", "reference=docker.io/kanthorlabs/sag", "--format", "{{.Repository}}:{{.Tag}}")
	if err != nil {
		return fmt.Errorf("precondition: image ls: %w", err)
	}
	if strings.TrimSpace(string(out)) != "" {
		return fmt.Errorf("precondition: image docker.io/kanthorlabs/sag exists")
	}
	return nil
}

func checkSecrets(ctx context.Context, r Runner) error {
	for _, name := range []string{"sag-master-key", "sag-tls-cert", "sag-tls-key", "sag-aws-credentials"} {
		out, err := r.Output(ctx, "podman", "secret", "ls", "--filter", "name=^"+name+"$", "--format", "{{.Name}}")
		if err != nil {
			return fmt.Errorf("precondition: secret ls %s: %w", name, err)
		}
		if strings.TrimSpace(string(out)) != "" {
			return fmt.Errorf("precondition: secret %s exists", name)
		}
	}
	allOut, err := r.Output(ctx, "podman", "secret", "ls", "--format", "{{.Name}}")
	if err != nil {
		return fmt.Errorf("precondition: secret ls sag.resource: %w", err)
	}
	for _, sname := range parseLines(allOut) {
		labelOut, err := r.Output(ctx, "podman", "secret", "inspect", sname, "--format", `{{index .Spec.Labels "sag.resource"}}`)
		if err != nil {
			return fmt.Errorf("precondition: secret inspect %s: %w", sname, err)
		}
		if strings.TrimSpace(string(labelOut)) != "" {
			return fmt.Errorf("precondition: secrets with sag.resource label exist")
		}
	}
	return nil
}

func CheckPreconditionsWithRecovery(ctx context.Context, r Runner, cfg PreconditionConfig, teardown func(context.Context) error) error {
	err := CheckPreconditions(ctx, r, cfg)
	if err == nil {
		return nil
	}
	if !cfg.Marker {
		return err
	}
	if err2 := teardown(ctx); err2 != nil {
		return fmt.Errorf("precondition: recovery teardown: %w", err2)
	}
	return CheckPreconditions(ctx, r, cfg)
}
