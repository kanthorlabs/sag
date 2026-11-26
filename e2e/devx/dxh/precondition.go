package dxh

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

const minFreeGiB = 10

type PreconditionConfig struct {
	Home               string
	UID                string
	CfgDir             string
	PortHeld           bool
	CfgExists          bool
	LaunchAgentExists  bool
	Marker             bool
}

func CheckPreconditions(ctx context.Context, r Runner, cfg PreconditionConfig) error {
	if err := checkMachine(ctx, r); err != nil {
		return err
	}
	if err := checkFreeSpace(ctx, r); err != nil {
		return err
	}
	if err := checkContainers(ctx, r); err != nil {
		return err
	}
	if cfg.PortHeld {
		return fmt.Errorf("precondition: port 8443 is held")
	}
	if err := checkVolumeAndImage(ctx, r); err != nil {
		return err
	}
	if err := checkSecrets(ctx, r); err != nil {
		return err
	}
	if cfg.CfgExists {
		return fmt.Errorf("precondition: cfg directory exists: %s", cfg.CfgDir)
	}
	if cfg.LaunchAgentExists {
		return fmt.Errorf("precondition: LaunchAgent plist exists")
	}
	return nil
}

func checkFreeSpace(ctx context.Context, r Runner) error {
	return CheckFreeSpace(ctx, r)
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
	_, err = r.Output(ctx, "podman", "image", "inspect", "docker.io/kanthorlabs/sag")
	if err == nil {
		return fmt.Errorf("precondition: image docker.io/kanthorlabs/sag exists")
	}
	return nil
}

func checkSecrets(ctx context.Context, r Runner) error {
	for _, name := range []string{"sag-master-key", "sag-tls-cert", "sag-tls-key", "sag-aws-credentials"} {
		out, err := r.Output(ctx, "podman", "secret", "ls", "--filter", "name="+name, "--format", "{{.Name}}")
		if err != nil {
			return fmt.Errorf("precondition: secret ls %s: %w", name, err)
		}
		if strings.TrimSpace(string(out)) != "" {
			return fmt.Errorf("precondition: secret %s exists", name)
		}
	}
	out, err := r.Output(ctx, "podman", "secret", "ls", "--filter", "label=sag.resource=", "--format", "{{.Name}}")
	if err != nil {
		return fmt.Errorf("precondition: secret ls sag.resource: %w", err)
	}
	if strings.TrimSpace(string(out)) != "" {
		return fmt.Errorf("precondition: secrets with sag.resource label exist")
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
