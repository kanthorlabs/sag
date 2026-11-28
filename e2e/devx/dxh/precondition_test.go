package dxh_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kanthorlabs/sag/e2e/devx/dxh"
)

func goodMachineRunner() *fakeRunner {
	r := newFake()
	r.set("podman machine list --format {{.Name}} {{.Running}}", []byte("podman-machine-default true\n"), nil)
	r.set("podman machine ssh podman-machine-default df -Pk /var", []byte("Filesystem 1024-blocks Used Available Capacity Mounted\n/dev/vda1 102400000 1024000 90000000 2% /var\n"), nil)
	r.set("podman ps -a --filter name=^sag$ --format {{.Names}}", nil, nil)
	r.set("podman ps -a --filter name=^sag-dev$ --format {{.Names}}", nil, nil)
	r.set("podman ps -a --filter name=^sag-dev-pg$ --format {{.Names}}", nil, nil)
	r.set("podman volume inspect sag-state", nil, errors.New("no such volume"))
	r.set("podman image ls --filter reference=docker.io/kanthorlabs/sag --format {{.Repository}}:{{.Tag}}", nil, nil)
	r.set("podman secret ls --filter name=^sag-master-key$ --format {{.Name}}", nil, nil)
	r.set("podman secret ls --filter name=^sag-tls-cert$ --format {{.Name}}", nil, nil)
	r.set("podman secret ls --filter name=^sag-tls-key$ --format {{.Name}}", nil, nil)
	r.set("podman secret ls --filter name=^sag-aws-credentials$ --format {{.Name}}", nil, nil)
	r.set("podman secret ls --format {{.Name}}", nil, nil)
	return r
}

func TestRefusesSecondMachine(t *testing.T) {
	r := newFake()
	r.set("podman machine list --format {{.Name}} {{.Running}}", []byte("podman-machine-default true\npodman-machine-other true\n"), nil)

	cfg := dxh.PreconditionConfig{CfgDir: "/home/u/.config/sag"}
	err := dxh.CheckPreconditions(context.Background(), r, cfg)
	if err == nil {
		t.Fatal("expected error for two machines")
	}
	if !strings.Contains(err.Error(), "machine") {
		t.Errorf("error = %q, want it to mention 'machine'", err)
	}
}

func TestRefusesStoppedMachine(t *testing.T) {
	r := newFake()
	r.set("podman machine list --format {{.Name}} {{.Running}}", []byte("podman-machine-default false\n"), nil)

	cfg := dxh.PreconditionConfig{CfgDir: "/home/u/.config/sag"}
	err := dxh.CheckPreconditions(context.Background(), r, cfg)
	if err == nil {
		t.Fatal("expected error for stopped machine")
	}
	if !strings.Contains(err.Error(), "running") {
		t.Errorf("error = %q, want it to mention 'running'", err)
	}
}

func TestRefusesLowDisk(t *testing.T) {
	r := newFake()
	r.set("podman machine list --format {{.Name}} {{.Running}}", []byte("podman-machine-default true\n"), nil)
	r.set("podman machine ssh podman-machine-default df -Pk /var", []byte("Filesystem 1024-blocks Used Available Capacity Mounted\n/dev/vda1 102400000 1024000 5000000 95% /var\n"), nil)

	cfg := dxh.PreconditionConfig{CfgDir: "/home/u/.config/sag"}
	err := dxh.CheckPreconditions(context.Background(), r, cfg)
	if err == nil {
		t.Fatal("expected error for low disk")
	}
	if !strings.Contains(err.Error(), "free space") {
		t.Errorf("error = %q, want it to mention 'free space'", err)
	}
}

func TestRefusesContainerSag(t *testing.T) {
	r := goodMachineRunner()
	r.set("podman ps -a --filter name=^sag$ --format {{.Names}}", []byte("sag\n"), nil)

	cfg := dxh.PreconditionConfig{CfgDir: "/home/u/.config/sag"}
	err := dxh.CheckPreconditions(context.Background(), r, cfg)
	if err == nil {
		t.Fatal("expected error for container sag")
	}
	if !strings.Contains(err.Error(), "sag") {
		t.Errorf("error = %q, want it to mention 'sag'", err)
	}
}

func TestRefusesDevContainers(t *testing.T) {
	r := goodMachineRunner()
	r.set("podman ps -a --filter name=^sag-dev$ --format {{.Names}}", []byte("sag-dev\n"), nil)

	cfg := dxh.PreconditionConfig{CfgDir: "/home/u/.config/sag"}
	err := dxh.CheckPreconditions(context.Background(), r, cfg)
	if err == nil {
		t.Fatal("expected error for sag-dev container")
	}
}

func TestRefusesPortHeld(t *testing.T) {
	r := goodMachineRunner()
	cfg := dxh.PreconditionConfig{
		CfgDir:   "/home/u/.config/sag",
		PortHeld: func() bool { return true },
	}
	err := dxh.CheckPreconditions(context.Background(), r, cfg)
	if err == nil {
		t.Fatal("expected error for port held")
	}
	if !strings.Contains(err.Error(), "8443") {
		t.Errorf("error = %q, want it to mention '8443'", err)
	}
}

func TestRefusesCfgOrLaunchAgent(t *testing.T) {
	r := goodMachineRunner()
	cfg := dxh.PreconditionConfig{
		CfgDir:    "/home/u/.config/sag",
		CfgExists: func() bool { return true },
	}
	err := dxh.CheckPreconditions(context.Background(), r, cfg)
	if err == nil {
		t.Fatal("expected error for existing cfg")
	}
	if !strings.Contains(err.Error(), "cfg") {
		t.Errorf("error = %q, want it to mention 'cfg'", err)
	}
}

func TestRefusesSAGSecretsOrVolume(t *testing.T) {
	r := goodMachineRunner()
	r.set("podman secret ls --filter name=^sag-master-key$ --format {{.Name}}", []byte("sag-master-key\n"), nil)

	cfg := dxh.PreconditionConfig{CfgDir: "/home/u/.config/sag"}
	err := dxh.CheckPreconditions(context.Background(), r, cfg)
	if err == nil {
		t.Fatal("expected error for sag-master-key secret")
	}
	if !strings.Contains(err.Error(), "sag-master-key") {
		t.Errorf("error = %q, want it to mention 'sag-master-key'", err)
	}
}

func TestMarkerRunsTeardownOnce(t *testing.T) {
	r := newFake()
	r.set("podman machine list --format {{.Name}} {{.Running}}", []byte("podman-machine-default true\n"), nil)
	r.set("podman machine ssh podman-machine-default df -Pk /var", []byte("Filesystem 1024-blocks Used Available Capacity Mounted\n/dev/vda1 102400000 1024000 90000000 2% /var\n"), nil)
	r.set("podman ps -a --filter name=^sag$ --format {{.Names}}", []byte("sag\n"), nil)

	teardownCalled := 0
	teardown := func(ctx context.Context) error {
		teardownCalled++
		r.set("podman ps -a --filter name=^sag$ --format {{.Names}}", nil, nil)
		r.set("podman ps -a --filter name=^sag-dev$ --format {{.Names}}", nil, nil)
		r.set("podman ps -a --filter name=^sag-dev-pg$ --format {{.Names}}", nil, nil)
		r.set("podman volume inspect sag-state", nil, errors.New("no such volume"))
		r.set("podman image ls --filter reference=docker.io/kanthorlabs/sag --format {{.Repository}}:{{.Tag}}", nil, nil)
		r.set("podman secret ls --filter name=^sag-master-key$ --format {{.Name}}", nil, nil)
		r.set("podman secret ls --filter name=^sag-tls-cert$ --format {{.Name}}", nil, nil)
		r.set("podman secret ls --filter name=^sag-tls-key$ --format {{.Name}}", nil, nil)
		r.set("podman secret ls --filter name=^sag-aws-credentials$ --format {{.Name}}", nil, nil)
		r.set("podman secret ls --format {{.Name}}", nil, nil)
		return nil
	}

	cfg := dxh.PreconditionConfig{CfgDir: "/home/u/.config/sag", Marker: true}
	err := dxh.CheckPreconditionsWithRecovery(context.Background(), r, cfg, teardown)
	if err != nil {
		t.Fatalf("expected no error after recovery: %v", err)
	}
	if teardownCalled != 1 {
		t.Errorf("teardown called %d times, want 1", teardownCalled)
	}
}

func TestNoMarkerChangesNothing(t *testing.T) {
	r := newFake()
	r.set("podman machine list --format {{.Name}} {{.Running}}", []byte("podman-machine-default true\n"), nil)
	r.set("podman machine ssh podman-machine-default df -Pk /var", []byte("Filesystem 1024-blocks Used Available Capacity Mounted\n/dev/vda1 102400000 1024000 90000000 2% /var\n"), nil)
	r.set("podman ps -a --filter name=^sag$ --format {{.Names}}", []byte("sag\n"), nil)

	teardownCalled := 0
	teardown := func(ctx context.Context) error {
		teardownCalled++
		return nil
	}

	cfg := dxh.PreconditionConfig{CfgDir: "/home/u/.config/sag", Marker: false}
	err := dxh.CheckPreconditionsWithRecovery(context.Background(), r, cfg, teardown)
	if err == nil {
		t.Fatal("expected error without marker")
	}
	if teardownCalled != 0 {
		t.Errorf("teardown called %d times, want 0", teardownCalled)
	}
}

func TestNoPodmanMachineCommandRuns(t *testing.T) {
	r, cfg := newCleanTeardownFake("/home/u", "1000", "/home/u/.config/sag")

	if err := dxh.Teardown(context.Background(), r, cfg); err != nil {
		t.Fatal(err)
	}

	forbidden := []string{"init", "start", "stop", "rm", "prune", "reset"}
	for _, call := range r.calls {
		if len(call) < 2 || call[0] != "podman" || call[1] != "machine" {
			continue
		}
		for _, f := range forbidden {
			if len(call) > 2 && call[2] == f {
				t.Errorf("teardown ran forbidden 'podman machine %s': %v", f, call)
			}
		}
	}
}

func TestRecoveryReadsStateAgain(t *testing.T) {
	r := goodMachineRunner()

	held := true
	cfg := dxh.PreconditionConfig{
		CfgDir:   "/home/u/.config/sag",
		Marker:   true,
		PortHeld: func() bool { return held },
	}

	teardown := func(ctx context.Context) error {
		held = false
		return nil
	}

	err := dxh.CheckPreconditionsWithRecovery(context.Background(), r, cfg, teardown)
	if err != nil {
		t.Fatalf("expected nil after recovery cleared port: %v", err)
	}
}

func TestRefusesLaunchAgent(t *testing.T) {
	r := goodMachineRunner()
	cfg := dxh.PreconditionConfig{
		CfgDir:            "/home/u/.config/sag",
		LaunchAgentExists: func() bool { return true },
	}
	err := dxh.CheckPreconditions(context.Background(), r, cfg)
	if err == nil {
		t.Fatal("expected error for existing LaunchAgent plist")
	}
	if !strings.Contains(err.Error(), "LaunchAgent") {
		t.Errorf("error = %q, want it to mention 'LaunchAgent'", err)
	}
}

func TestRefusesVolume(t *testing.T) {
	r := goodMachineRunner()
	r.set("podman volume inspect sag-state", nil, nil)

	cfg := dxh.PreconditionConfig{CfgDir: "/home/u/.config/sag"}
	err := dxh.CheckPreconditions(context.Background(), r, cfg)
	if err == nil {
		t.Fatal("expected error for existing sag-state volume")
	}
	if !strings.Contains(err.Error(), "sag-state") {
		t.Errorf("error = %q, want it to mention 'sag-state'", err)
	}
}

func TestRefusesImage(t *testing.T) {
	r := goodMachineRunner()
	r.set("podman image ls --filter reference=docker.io/kanthorlabs/sag --format {{.Repository}}:{{.Tag}}", []byte("docker.io/kanthorlabs/sag:main\n"), nil)

	cfg := dxh.PreconditionConfig{CfgDir: "/home/u/.config/sag"}
	err := dxh.CheckPreconditions(context.Background(), r, cfg)
	if err == nil {
		t.Fatal("expected error for existing image")
	}
	if !strings.Contains(err.Error(), "image") {
		t.Errorf("error = %q, want it to mention 'image'", err)
	}
}

func TestRefusesSagResourceLabel(t *testing.T) {
	r := goodMachineRunner()
	r.set("podman secret ls --format {{.Name}}", []byte("sag-res-canary.conn\n"), nil)
	r.set(`podman secret inspect sag-res-canary.conn --format {{index .Spec.Labels "sag.resource"}}`, []byte("true"), nil)

	cfg := dxh.PreconditionConfig{CfgDir: "/home/u/.config/sag"}
	err := dxh.CheckPreconditions(context.Background(), r, cfg)
	if err == nil {
		t.Fatal("expected error for secret with sag.resource label")
	}
	if !strings.Contains(err.Error(), "sag.resource") {
		t.Errorf("error = %q, want it to mention 'sag.resource'", err)
	}
}

func TestAnchoredSecretNameFilter(t *testing.T) {
	r := goodMachineRunner()
	cfg := dxh.PreconditionConfig{CfgDir: "/home/u/.config/sag"}
	if err := dxh.CheckPreconditions(context.Background(), r, cfg); err != nil {
		t.Fatal(err)
	}
	for _, call := range r.calls {
		joined := strings.Join(call, " ")
		if !strings.Contains(joined, "podman secret ls --filter name=") {
			continue
		}
		if strings.Contains(joined, "name=sag") && !strings.Contains(joined, "name=^sag") {
			t.Errorf("secret name filter missing anchors: %v", call)
		}
	}
}

func goodLinuxRunner() *fakeRunner {
	r := newFake()
	r.set("podman info --format {{.Store.GraphRoot}}", []byte("/home/u/.local/share/containers/storage\n"), nil)
	r.set("df -Pk /home/u/.local/share/containers/storage", []byte("Filesystem 1024-blocks Used Available Capacity Mounted\n/dev/sda1 102400000 1024000 90000000 2% /\n"), nil)
	r.set("podman ps -a --filter name=^sag$ --format {{.Names}}", nil, nil)
	r.set("podman ps -a --filter name=^sag-dev$ --format {{.Names}}", nil, nil)
	r.set("podman ps -a --filter name=^sag-dev-pg$ --format {{.Names}}", nil, nil)
	r.set("podman volume inspect sag-state", nil, errors.New("no such volume"))
	r.set("podman image ls --filter reference=docker.io/kanthorlabs/sag --format {{.Repository}}:{{.Tag}}", nil, nil)
	r.set("podman secret ls --filter name=^sag-master-key$ --format {{.Name}}", nil, nil)
	r.set("podman secret ls --filter name=^sag-tls-cert$ --format {{.Name}}", nil, nil)
	r.set("podman secret ls --filter name=^sag-tls-key$ --format {{.Name}}", nil, nil)
	r.set("podman secret ls --filter name=^sag-aws-credentials$ --format {{.Name}}", nil, nil)
	r.set("podman secret ls --format {{.Name}}", nil, nil)
	r.set("systemctl --user is-active sag.service", nil, errors.New("inactive"))
	return r
}

func TestLinuxPreconditions(t *testing.T) {
	r := goodLinuxRunner()
	cfg := dxh.PreconditionConfig{CfgDir: "/home/u/.config/sag"}
	if err := dxh.CheckLinuxPreconditions(context.Background(), r, cfg); err != nil {
		t.Fatalf("expected no error: %v", err)
	}
	for _, call := range r.calls {
		if len(call) >= 2 && call[0] == "podman" && call[1] == "machine" {
			t.Errorf("Linux preconditions called 'podman machine': %v", call)
		}
	}
}

func TestLinuxRefusesLowDisk(t *testing.T) {
	r := goodLinuxRunner()
	r.set("df -Pk /home/u/.local/share/containers/storage", []byte("Filesystem 1024-blocks Used Available Capacity Mounted\n/dev/sda1 102400000 97400000 5000000 95% /\n"), nil)

	cfg := dxh.PreconditionConfig{CfgDir: "/home/u/.config/sag"}
	err := dxh.CheckLinuxPreconditions(context.Background(), r, cfg)
	if err == nil {
		t.Fatal("expected error for low disk")
	}
	if !strings.Contains(err.Error(), "free space") {
		t.Errorf("error = %q, want it to mention 'free space'", err)
	}
}
