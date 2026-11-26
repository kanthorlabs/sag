package dxh

import (
	"context"
	"fmt"
	"strings"
)

type TeardownConfig struct {
	Home string
	UID  string
}

func Teardown(ctx context.Context, r Runner, cfg TeardownConfig) error {
	if err := removeResourceSecrets(ctx, r); err != nil {
		return fmt.Errorf("teardown: resource secrets: %w", err)
	}
	if err := removeLabeledObjects(ctx, r); err != nil {
		return fmt.Errorf("teardown: labeled objects: %w", err)
	}
	if err := macOSBootout(ctx, r, cfg.UID); err != nil {
		return fmt.Errorf("teardown: launchctl bootout: %w", err)
	}
	if _, err := r.Output(ctx, "podman", "rm", "-f", "sag"); err != nil {
		return fmt.Errorf("teardown: podman rm sag: %w", err)
	}
	if _, err := r.Output(ctx, "podman", "volume", "rm", "sag-state"); err != nil {
		return fmt.Errorf("teardown: podman volume rm sag-state: %w", err)
	}
	if _, err := r.Output(ctx, "podman", "secret", "rm", "sag-master-key", "sag-tls-cert", "sag-tls-key"); err != nil {
		return fmt.Errorf("teardown: podman secret rm: %w", err)
	}
	if err := removeSAGImages(ctx, r); err != nil {
		return fmt.Errorf("teardown: podman rmi: %w", err)
	}
	return nil
}

func removeResourceSecrets(ctx context.Context, r Runner) error {
	out, err := r.Output(ctx, "podman", "secret", "ls", "--filter", "label=sag.resource=", "--format", "{{.Name}}")
	if err != nil {
		return err
	}
	names := parseLines(out)
	for _, name := range names {
		if _, err := r.Output(ctx, "podman", "secret", "rm", name); err != nil {
			return err
		}
	}
	return nil
}

func removeLabeledObjects(ctx context.Context, r Runner) error {
	containers, err := r.Output(ctx, "podman", "ps", "-a", "--filter", "label=sag.devx-e2e=1", "--format", "{{.Names}}")
	if err != nil {
		return err
	}
	for _, name := range parseLines(containers) {
		if _, err := r.Output(ctx, "podman", "rm", "-f", name); err != nil {
			return err
		}
	}
	volumes, err := r.Output(ctx, "podman", "volume", "ls", "--filter", "label=sag.devx-e2e=1", "--format", "{{.Name}}")
	if err != nil {
		return err
	}
	for _, name := range parseLines(volumes) {
		if _, err := r.Output(ctx, "podman", "volume", "rm", name); err != nil {
			return err
		}
	}
	secrets, err := r.Output(ctx, "podman", "secret", "ls", "--filter", "label=sag.devx-e2e=1", "--format", "{{.Name}}")
	if err != nil {
		return err
	}
	for _, name := range parseLines(secrets) {
		if _, err := r.Output(ctx, "podman", "secret", "rm", name); err != nil {
			return err
		}
	}
	networks, err := r.Output(ctx, "podman", "network", "ls", "--filter", "label=sag.devx-e2e=1", "--format", "{{.Name}}")
	if err != nil {
		return err
	}
	for _, name := range parseLines(networks) {
		if _, err := r.Output(ctx, "podman", "network", "rm", name); err != nil {
			return err
		}
	}
	return nil
}

func macOSBootout(ctx context.Context, r Runner, uid string) error {
	_, _ = r.Output(ctx, "launchctl", "bootout", "gui/"+uid, "net.kanthorlabs.sag")
	return nil
}

func removeSAGImages(ctx context.Context, r Runner) error {
	out, err := r.Output(ctx, "podman", "image", "ls", "--filter", "reference=docker.io/kanthorlabs/sag", "--format", "{{.Repository}}:{{.Tag}}")
	if err != nil {
		return err
	}
	for _, ref := range parseLines(out) {
		if ref == "localhost/sag:devx-e2e" {
			continue
		}
		if _, err := r.Output(ctx, "podman", "rmi", ref); err != nil {
			return err
		}
	}
	return nil
}

func parseLines(b []byte) []string {
	var out []string
	for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}
