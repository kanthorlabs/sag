//go:build linux

package service

import (
	"context"
	"crypto/tls"
	"crypto/x509"
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
	"github.com/kanthorlabs/sag/internal/hoststate"
	"github.com/kanthorlabs/sag/internal/podman"
	"github.com/kanthorlabs/sag/internal/stepline"
)

const (
	logTailLines    = 50
	healthzDeadline = 30 * time.Second
)

type linuxDeps struct {
	systemctlRun func(ctx context.Context, args ...string) ([]byte, error)
	healthzCheck func(ctx context.Context, caPath string) error
	logTail      func(ctx context.Context, n int) ([]byte, error)
	portCheck    func(ctx context.Context, pm podman.Command, out stepline.Writer) error
	interval     time.Duration
	timeout      time.Duration
}

func defaultLinuxDeps() linuxDeps {
	return linuxDeps{
		systemctlRun: realSystemctl,
		healthzCheck: realHealthzCheck,
		logTail:      realLinuxLogTail,
		portCheck:    checkPort,
		interval:     time.Second,
		timeout:      healthzDeadline,
	}
}

func realSystemctl(ctx context.Context, args ...string) ([]byte, error) {
	out, err := exec.CommandContext(ctx, "systemctl", args...).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("systemctl %v: %w", args, err)
	}
	return out, nil
}

func realLinuxLogTail(ctx context.Context, n int) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "journalctl", "--user", "-u", "sag", "-n", fmt.Sprintf("%d", n))
	return cmd.CombinedOutput()
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

func quadletPath(home string) string {
	return filepath.Join(home, ".config", "containers", "systemd", "sag.container")
}

func writeQuadlet(qPath string, sagCfg *config.Config, cfg Config, autostart bool) error {
	content := deploy.Quadlet(sagCfg, deploy.RunOpts{
		Image:         cfg.Image,
		ConfigSource:  filepath.Join(cfg.CfgDir, "sag.yaml"),
		Autostart:     autostart,
		RestartPolicy: "on-failure",
	})
	if err := os.MkdirAll(filepath.Dir(qPath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(qPath, []byte(content), 0o644)
}

func Start(ctx context.Context, cfg Config, out stepline.Writer) error {
	return start(ctx, cfg, defaultLinuxDeps(), out)
}

func start(ctx context.Context, cfg Config, d linuxDeps, out stepline.Writer) error {
	if d.portCheck != nil {
		if err := d.portCheck(ctx, cfg.Podman, out); err != nil {
			return err
		}
	}

	sagCfg, err := config.Load(filepath.Join(cfg.CfgDir, "sag.yaml"))
	if err != nil {
		return fmt.Errorf("load sag.yaml: %w", err)
	}

	answers, err := hoststate.LoadAnswers(hoststate.Paths{AnswersJSON: filepath.Join(cfg.CfgDir, "answers.json")})
	if err != nil {
		return fmt.Errorf("load answers.json: %w", err)
	}
	autostart := answers.Autostart == "yes"

	qPath := quadletPath(cfg.Home)
	if err := writeQuadlet(qPath, sagCfg, cfg, autostart); err != nil {
		return fmt.Errorf("write quadlet: %w", err)
	}

	if _, err := d.systemctlRun(ctx, "--user", "daemon-reload"); err != nil {
		return fmt.Errorf("systemctl daemon-reload: %w", err)
	}
	if _, err := d.systemctlRun(ctx, "--user", "restart", "sag"); err != nil {
		return fmt.Errorf("systemctl restart sag: %w", err)
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

func Stop(ctx context.Context, cfg Config, out stepline.Writer) error {
	return stop(ctx, cfg, defaultLinuxDeps(), out)
}

func stop(ctx context.Context, cfg Config, d linuxDeps, out stepline.Writer) error {
	if _, err := d.systemctlRun(ctx, "--user", "stop", "sag"); err != nil {
		return fmt.Errorf("systemctl stop sag: %w", err)
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
	return applyAutostart(ctx, cfg, enable, defaultLinuxDeps(), out)
}

func applyAutostart(ctx context.Context, cfg Config, enable bool, d linuxDeps, out stepline.Writer) (bool, error) {
	sagCfg, err := config.Load(filepath.Join(cfg.CfgDir, "sag.yaml"))
	if err != nil {
		return false, fmt.Errorf("load sag.yaml: %w", err)
	}

	qPath := quadletPath(cfg.Home)
	newContent := deploy.Quadlet(sagCfg, deploy.RunOpts{
		Image:         cfg.Image,
		ConfigSource:  filepath.Join(cfg.CfgDir, "sag.yaml"),
		Autostart:     enable,
		RestartPolicy: "on-failure",
	})

	existing, err := os.ReadFile(qPath)
	if err == nil && string(existing) == newContent {
		out.Line(stepline.StatusOK, "autostart", "")
		return false, nil
	}

	if err := os.MkdirAll(filepath.Dir(qPath), 0o755); err != nil {
		return false, err
	}
	if err := os.WriteFile(qPath, []byte(newContent), 0o644); err != nil {
		return false, err
	}

	if _, err := d.systemctlRun(ctx, "--user", "daemon-reload"); err != nil {
		return false, fmt.Errorf("systemctl daemon-reload: %w", err)
	}

	out.Line(stepline.StatusInstall, "autostart", "")
	return true, nil
}
