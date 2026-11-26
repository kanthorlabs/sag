package dxh

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type TeardownConfig struct {
	Home   string
	UID    string
	CfgDir string
}

func Teardown(ctx context.Context, r Runner, cfg TeardownConfig) error {
	if err := checkTeardownPaths(cfg); err != nil {
		return err
	}
	var errs []string

	if err := removeResourceSecrets(ctx, r); err != nil {
		errs = append(errs, fmt.Sprintf("resource secrets: %s", err))
	}
	if err := removeLabeledObjects(ctx, r); err != nil {
		errs = append(errs, fmt.Sprintf("labeled objects: %s", err))
	}
	if err := macOSBootout(ctx, r, cfg.UID); err != nil {
		errs = append(errs, fmt.Sprintf("launchctl bootout: %s", err))
	}
	if _, err := r.Output(ctx, "rm", "-f", cfg.Home+"/Library/LaunchAgents/net.kanthorlabs.sag.plist"); err != nil {
		errs = append(errs, fmt.Sprintf("rm plist: %s", err))
	}
	if _, err := r.Output(ctx, "rm", "-rf", cfg.Home+"/Library/Logs/sag"); err != nil {
		errs = append(errs, fmt.Sprintf("rm logs: %s", err))
	}
	if _, err := r.Output(ctx, "podman", "rm", "-f", "sag"); err != nil {
		errs = append(errs, fmt.Sprintf("podman rm sag: %s", err))
	}
	if err := removeVolumeIfPresent(ctx, r, "sag-state"); err != nil {
		errs = append(errs, fmt.Sprintf("podman volume rm sag-state: %s", err))
	}
	for _, name := range []string{"sag-master-key", "sag-tls-cert", "sag-tls-key", "sag-aws-credentials"} {
		if err := removeSecretIfPresent(ctx, r, name); err != nil {
			errs = append(errs, fmt.Sprintf("podman secret rm %s: %s", name, err))
		}
	}
	if err := removeSAGImages(ctx, r); err != nil {
		errs = append(errs, fmt.Sprintf("podman rmi: %s", err))
	}
	if cfg.CfgDir != "" {
		_, statErr := os.Stat(cfg.CfgDir)
		if _, err := r.Output(ctx, "rm", "-rf", cfg.CfgDir); err != nil {
			errs = append(errs, fmt.Sprintf("rm cfg: %s", err))
		} else if statErr == nil {
			if err := waitMachineSeesAbsent(ctx, r, cfg.CfgDir, machineSyncTimeout); err != nil {
				errs = append(errs, err.Error())
			}
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("teardown: %s", strings.Join(errs, "; "))
	}
	return nil
}

const machineSyncTimeout = 15 * time.Second

func waitMachineSeesAbsent(ctx context.Context, r Runner, path string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		_, err := r.Output(ctx, "podman", "machine", "ssh", "--", "test", "-e", path)
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("podman machine still sees %s after %s", path, timeout)
		}
		time.Sleep(250 * time.Millisecond)
	}
}

func removeResourceSecrets(ctx context.Context, r Runner) error {
	out, err := r.Output(ctx, "podman", "secret", "ls", "--format", "{{.Name}}")
	if err != nil {
		return err
	}
	for _, name := range parseLines(out) {
		labelOut, err := r.Output(ctx, "podman", "secret", "inspect", name, "--format", `{{index .Spec.Labels "sag.resource"}}`)
		if err != nil {
			return err
		}
		if strings.TrimSpace(string(labelOut)) == "" {
			continue
		}
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
	secrets, err := r.Output(ctx, "podman", "secret", "ls", "--format", "{{.Name}}")
	if err != nil {
		return err
	}
	for _, name := range parseLines(secrets) {
		labelOut, err := r.Output(ctx, "podman", "secret", "inspect", name, "--format", `{{index .Spec.Labels "sag.devx-e2e"}}`)
		if err != nil {
			return err
		}
		if strings.TrimSpace(string(labelOut)) != "1" {
			continue
		}
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
	target := "gui/" + uid + "/net.kanthorlabs.sag"
	if _, err := r.Output(ctx, "launchctl", "print", target); err != nil {
		return nil
	}
	_, err := r.Output(ctx, "launchctl", "bootout", target)
	return err
}

func checkTeardownPaths(cfg TeardownConfig) error {
	if !filepath.IsAbs(cfg.Home) || filepath.Clean(cfg.Home) == "/" {
		return fmt.Errorf("teardown: home %q is not an absolute user directory", cfg.Home)
	}
	if cfg.CfgDir != "" && (!filepath.IsAbs(cfg.CfgDir) || filepath.Base(cfg.CfgDir) != "sag") {
		return fmt.Errorf("teardown: config directory %q is not an absolute sag directory", cfg.CfgDir)
	}
	return nil
}

func removeVolumeIfPresent(ctx context.Context, r Runner, name string) error {
	out, err := r.Output(ctx, "podman", "volume", "ls", "--filter", "name=^"+name+"$", "--format", "{{.Name}}")
	if err != nil {
		return err
	}
	if strings.TrimSpace(string(out)) == "" {
		return nil
	}
	_, err = r.Output(ctx, "podman", "volume", "rm", name)
	return err
}

func removeSecretIfPresent(ctx context.Context, r Runner, name string) error {
	out, err := r.Output(ctx, "podman", "secret", "ls", "--filter", "name=^"+name+"$", "--format", "{{.Name}}")
	if err != nil {
		return err
	}
	if strings.TrimSpace(string(out)) == "" {
		return nil
	}
	_, err = r.Output(ctx, "podman", "secret", "rm", name)
	return err
}

func removeSAGImages(ctx context.Context, r Runner) error {
	out, err := r.Output(ctx, "podman", "image", "ls", "--filter", "reference=docker.io/kanthorlabs/sag", "--format", "{{.Repository}}:{{.Tag}}")
	if err != nil {
		return err
	}
	for _, ref := range parseLines(out) {
		if !strings.HasPrefix(ref, "docker.io/kanthorlabs/sag:") && !strings.HasPrefix(ref, "docker.io/kanthorlabs/sag@") {
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
