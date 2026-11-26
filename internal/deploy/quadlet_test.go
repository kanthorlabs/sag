package deploy_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kanthorlabs/sag/internal/config"
	"github.com/kanthorlabs/sag/internal/deploy"
)

func loadDevCfg(t *testing.T) *config.Config {
	t.Helper()
	cfg, err := config.Load(filepath.Join("..", "..", "deploy", "dev", "sag.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func devOpts() deploy.RunOpts {
	return deploy.RunOpts{
		Image:         "localhost/sag:latest",
		ConfigSource:  "/etc/sag/sag.yaml",
		Autostart:     true,
		RestartPolicy: "on-failure",
	}
}

func TestQuadletGolden(t *testing.T) {
	cfg := loadDevCfg(t)
	got := deploy.Quadlet(cfg, devOpts())
	want, err := os.ReadFile(filepath.Join("..", "..", "deploy", "sag.container"))
	if err != nil {
		t.Fatal(err)
	}
	if got != string(want) {
		t.Errorf("Quadlet output:\n%s\nwant:\n%s", got, want)
	}
}

func TestQuadletLogKeys(t *testing.T) {
	cfg := loadDevCfg(t)
	got := deploy.Quadlet(cfg, devOpts())
	if !strings.Contains(got, "LogDriver=k8s-file\n") {
		t.Error("Quadlet output has no LogDriver=k8s-file")
	}
	if !strings.Contains(got, "PodmanArgs=--log-opt=max-size=10mb\n") {
		t.Error("Quadlet output has no PodmanArgs=--log-opt=max-size=10mb")
	}
}

func TestQuadletAutostartInstallSection(t *testing.T) {
	cfg := loadDevCfg(t)

	with := deploy.Quadlet(cfg, deploy.RunOpts{
		Image: "i", ConfigSource: "c",
		Autostart: true, RestartPolicy: "on-failure",
	})
	if !strings.Contains(with, "[Install]\nWantedBy=default.target\n") {
		t.Error("Quadlet with Autostart=true has no [Install] section")
	}

	without := deploy.Quadlet(cfg, deploy.RunOpts{
		Image: "i", ConfigSource: "c",
		Autostart: false, RestartPolicy: "on-failure",
	})
	if strings.Contains(without, "[Install]") {
		t.Error("Quadlet with Autostart=false has [Install] section")
	}
}
