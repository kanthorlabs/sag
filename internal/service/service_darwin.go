//go:build darwin

package service

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/kanthorlabs/sag/internal/config"
	"github.com/kanthorlabs/sag/internal/deploy"
	"github.com/kanthorlabs/sag/internal/podman"
	"github.com/kanthorlabs/sag/internal/stepline"
)

const (
	launchAgentLabel = "net.kanthorlabs.sag"
	logTailLines     = 50
	healthzDeadline  = 30 * time.Second
)

type deps struct {
	launchctlRun  func(ctx context.Context, args ...string) ([]byte, error)
	healthzCheck  func(ctx context.Context, caPath string) error
	logTail       func(ctx context.Context, n int) ([]byte, error)
	portCheck     func(ctx context.Context, pm podman.Command, out stepline.Writer) error
	configVisible func(ctx context.Context, pm podman.Command, path string) error
	interval      time.Duration
	timeout       time.Duration
}

func defaultDeps(cfg Config) deps {
	return deps{
		launchctlRun: realLaunchctl,
		healthzCheck: realHealthzCheck,
		logTail:      makeLogTail(cfg),
		portCheck:    checkPort,
		configVisible: func(ctx context.Context, pm podman.Command, path string) error {
			return waitMachineSees(ctx, pm, path, machineSyncTimeout, machineSyncInterval)
		},
		interval: time.Second,
		timeout:  healthzDeadline,
	}
}

func makeLogTail(cfg Config) func(ctx context.Context, n int) ([]byte, error) {
	runner, ok := cfg.Podman.(podman.Runner)
	if ok {
		return func(ctx context.Context, n int) ([]byte, error) {
			cmd := exec.CommandContext(ctx, runner.Path, "logs", "--tail", fmt.Sprintf("%d", n), "sag")
			return cmd.CombinedOutput()
		}
	}
	return func(ctx context.Context, n int) ([]byte, error) {
		return cfg.Podman.Run(ctx, nil, "logs", "--tail", fmt.Sprintf("%d", n), "sag")
	}
}

func realLaunchctl(ctx context.Context, args ...string) ([]byte, error) {
	out, err := exec.CommandContext(ctx, "launchctl", args...).Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return nil, &launchctlError{code: exitErr.ExitCode(), stderr: strings.TrimSpace(string(exitErr.Stderr))}
		}
		return nil, err
	}
	return out, nil
}

type launchctlError struct {
	code   int
	stderr string
}

func (e *launchctlError) Error() string {
	return fmt.Sprintf("launchctl exit %d: %s", e.code, e.stderr)
}

func realHealthzCheck(ctx context.Context, caPath string) error {
	pem, err := os.ReadFile(caPath)
	if err != nil {
		return err
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pem) {
		return fmt.Errorf("ca.pem: no certificate found")
	}
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{RootCAs: pool},
		},
		Timeout: 5 * time.Second,
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
		return fmt.Errorf("healthz: %d", resp.StatusCode)
	}
	return nil
}

type machineInfo struct {
	Name    string `json:"Name"`
	Running bool   `json:"Running"`
}

func Start(ctx context.Context, cfg Config, out stepline.Writer) error {
	return start(ctx, cfg, defaultDeps(cfg), out)
}

func start(ctx context.Context, cfg Config, d deps, out stepline.Writer) error {
	if err := ensureMachineRunning(ctx, cfg); err != nil {
		return err
	}
	if d.portCheck != nil {
		if err := d.portCheck(ctx, cfg.Podman, out); err != nil {
			return err
		}
	}

	sagCfg, err := config.Load(filepath.Join(cfg.CfgDir, "sag.yaml"))
	if err != nil {
		return fmt.Errorf("load sag.yaml: %w", err)
	}

	if d.configVisible != nil {
		if err := d.configVisible(ctx, cfg.Podman, filepath.Join(cfg.CfgDir, "sag.yaml")); err != nil {
			return err
		}
	}

	runArgs := deploy.RunArgs(sagCfg, deploy.RunOpts{
		Image:         cfg.Image,
		ConfigSource:  filepath.Join(cfg.CfgDir, "sag.yaml"),
		RestartPolicy: "on-failure",
	})

	if _, err := cfg.Podman.Run(ctx, nil, runArgs...); err != nil {
		return fmt.Errorf("podman run: %w", err)
	}

	caPath := filepath.Join(cfg.CfgDir, "ca.pem")
	deadline := time.Now().Add(d.timeout)
	for {
		if err := d.healthzCheck(ctx, caPath); err == nil {
			out.Line(stepline.StatusOK, "sag", "")
			return nil
		}
		if time.Now().After(deadline) {
			tail, _ := d.logTail(ctx, logTailLines)
			return fmt.Errorf("sag health check timed out:\n%s", strings.TrimSpace(string(tail)))
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(d.interval):
		}
	}
}

const (
	machineSyncTimeout  = 15 * time.Second
	machineSyncInterval = 250 * time.Millisecond
)

func waitMachineSees(ctx context.Context, pm podman.Command, path string, timeout, interval time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		if _, err := pm.Run(ctx, nil, "machine", "ssh", "--", "test", "-f", path); err == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("podman machine does not see %s after %s", path, timeout)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(interval):
		}
	}
}

func ensureMachineRunning(ctx context.Context, cfg Config) error {
	data, err := cfg.Podman.Run(ctx, nil, "machine", "list", "--format", "json")
	if err != nil {
		return fmt.Errorf("machine list: %w", err)
	}
	var machines []machineInfo
	if err := json.Unmarshal(data, &machines); err != nil {
		return fmt.Errorf("machine list parse: %w", err)
	}
	for _, m := range machines {
		if m.Name == "podman-machine-default" {
			if m.Running {
				return nil
			}
			if _, err := cfg.Podman.Run(ctx, nil, "machine", "start"); err != nil {
				return fmt.Errorf("machine start: %w", err)
			}
			return nil
		}
	}
	return nil
}

func Stop(ctx context.Context, cfg Config, out stepline.Writer) error {
	if _, err := cfg.Podman.Run(ctx, nil, "stop", "sag"); err != nil {
		return fmt.Errorf("podman stop sag: %w", err)
	}
	out.Line(stepline.StatusOK, "sag", "")
	return nil
}

func Running(ctx context.Context, cfg Config) (bool, error) {
	out, err := cfg.Podman.Run(ctx, nil, "container", "inspect", "sag", "--format", "{{.State.Running}}")
	if err != nil {
		var runErr *podman.RunError
		if errors.As(err, &runErr) {
			return false, nil
		}
		return false, err
	}
	return strings.TrimSpace(string(out)) == "true", nil
}

func ApplyAutostart(ctx context.Context, cfg Config, enable bool, out stepline.Writer) (changed bool, err error) {
	return applyAutostart(ctx, cfg, enable, defaultDeps(cfg), out)
}

func applyAutostart(ctx context.Context, cfg Config, enable bool, d deps, out stepline.Writer) (bool, error) {
	plistPath := filepath.Join(cfg.Home, "Library", "LaunchAgents", launchAgentLabel+".plist")
	logDir := filepath.Join(cfg.Home, "Library", "Logs", "sag")
	target := "gui/" + cfg.UID + "/" + launchAgentLabel

	if !enable {
		_ = bootout(ctx, d, target)
		_, statErr := os.Stat(plistPath)
		if os.IsNotExist(statErr) {
			out.Line(stepline.StatusOK, "autostart", "")
			return false, nil
		}
		if err := os.Remove(plistPath); err != nil && !os.IsNotExist(err) {
			return false, err
		}
		out.Line(stepline.StatusInstall, "autostart", "")
		return true, nil
	}

	newPlist := buildPlist(cfg.BinPath, logDir)

	existing, err := os.ReadFile(plistPath)
	if err == nil && string(existing) == newPlist {
		out.Line(stepline.StatusOK, "autostart", "")
		return false, nil
	}

	if err := os.MkdirAll(filepath.Join(cfg.Home, "Library", "LaunchAgents"), 0o755); err != nil {
		return false, err
	}
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return false, err
	}
	if err := os.WriteFile(plistPath, []byte(newPlist), 0o644); err != nil {
		return false, err
	}

	_ = bootout(ctx, d, target)

	if _, err := d.launchctlRun(ctx, "print", "gui/"+cfg.UID); err != nil {
		out.Line(stepline.StatusWarn, "autostart", "loads at the next GUI login")
		return true, nil
	}

	if _, err := d.launchctlRun(ctx, "bootstrap", "gui/"+cfg.UID, plistPath); err != nil {
		return false, fmt.Errorf("launchctl bootstrap: %w", err)
	}

	out.Line(stepline.StatusInstall, "autostart", "")
	return true, nil
}

func bootout(ctx context.Context, d deps, target string) error {
	_, err := d.launchctlRun(ctx, "bootout", target)
	return err
}

func buildPlist(binPath, logDir string) string {
	logPath := filepath.Join(logDir, "service.log")
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>%s</string>
	<key>ProgramArguments</key>
	<array>
		<string>%s</string>
		<string>service</string>
		<string>start</string>
	</array>
	<key>RunAtLoad</key>
	<true/>
	<key>StandardOutPath</key>
	<string>%s</string>
	<key>StandardErrorPath</key>
	<string>%s</string>
</dict>
</plist>
`, launchAgentLabel, binPath, logPath, logPath)
}
