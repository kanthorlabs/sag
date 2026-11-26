package deploy_test

import (
	"slices"
	"testing"

	"github.com/kanthorlabs/sag/internal/config"
	"github.com/kanthorlabs/sag/internal/deploy"
	"github.com/kanthorlabs/sag/internal/secret"
)

func minCfg(t *testing.T) *config.Config {
	t.Helper()
	cfg, err := config.Load("../../deploy/dev/sag.yaml")
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func flagValues(args []string, flag string) []string {
	var vals []string
	for i, a := range args {
		if a == flag && i+1 < len(args) {
			vals = append(vals, args[i+1])
		}
	}
	return vals
}

func TestRunArgsLogFlags(t *testing.T) {
	args := deploy.RunArgs(minCfg(t), deploy.RunOpts{Image: "img", ConfigSource: "/etc/sag/sag.yaml"})

	drivers := flagValues(args, "--log-driver")
	if len(drivers) != 1 || drivers[0] != "k8s-file" {
		t.Errorf("--log-driver values = %v, want [k8s-file]", drivers)
	}

	opts := flagValues(args, "--log-opt")
	if len(opts) != 1 || opts[0] != "max-size=10mb" {
		t.Errorf("--log-opt values = %v, want [max-size=10mb]", opts)
	}
}

func TestRunArgsPublishLoopbackOnly(t *testing.T) {
	args := deploy.RunArgs(minCfg(t), deploy.RunOpts{Image: "img", ConfigSource: "/etc/sag/sag.yaml"})
	pubs := flagValues(args, "-p")
	if len(pubs) != 1 {
		t.Fatalf("-p appears %d times, want exactly 1", len(pubs))
	}
	if pubs[0] != "127.0.0.1:8443:8443" {
		t.Errorf("-p %q; want 127.0.0.1:8443:8443", pubs[0])
	}
}

func TestRunArgsReplace(t *testing.T) {
	args := deploy.RunArgs(minCfg(t), deploy.RunOpts{Image: "img", ConfigSource: "c"})
	if !slices.Contains(args, "--replace") {
		t.Errorf("RunArgs missing --replace: %v", args)
	}
}

func TestRunArgsRestartPolicy(t *testing.T) {
	args := deploy.RunArgs(minCfg(t), deploy.RunOpts{Image: "img", ConfigSource: "c", RestartPolicy: "on-failure"})
	found := false
	for _, a := range args {
		if a == "--restart=on-failure" {
			found = true
		}
	}
	if !found {
		t.Errorf("RunArgs missing --restart=on-failure: %v", args)
	}

	argsNoRestart := deploy.RunArgs(minCfg(t), deploy.RunOpts{Image: "img", ConfigSource: "c"})
	for _, a := range argsNoRestart {
		if len(a) > 9 && a[:9] == "--restart" {
			t.Errorf("RunArgs with empty RestartPolicy has --restart: %v", argsNoRestart)
		}
	}
}

func TestRunArgsAndQuadletUseSecretMounts(t *testing.T) {
	cfg := minCfg(t)
	mounts := deploy.SecretMounts(cfg)

	runArgs := deploy.RunArgs(cfg, deploy.RunOpts{Image: "img", ConfigSource: "c"})
	for _, name := range mounts {
		found := false
		for _, a := range runArgs {
			if len(a) >= len(name) && a[:len(name)] == name {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("RunArgs missing secret mount %q", name)
		}
	}

	quadlet := deploy.Quadlet(cfg, deploy.RunOpts{Image: "img", ConfigSource: "c", RestartPolicy: "on-failure"})
	for _, name := range mounts {
		if !slices.ContainsFunc([]string{quadlet}, func(s string) bool {
			return len(s) >= len(name) && containsStr(s, name)
		}) {
			t.Errorf("Quadlet missing secret mount %q", name)
		}
	}
}

func TestRunArgsMasterKeyFirst(t *testing.T) {
	cfg := minCfg(t)
	args := deploy.RunArgs(cfg, deploy.RunOpts{Image: "img", ConfigSource: "c"})
	firstSecretIdx := -1
	for i, a := range args {
		if a == "--secret" && i+1 < len(args) {
			firstSecretIdx = i + 1
			break
		}
	}
	if firstSecretIdx < 0 {
		t.Fatal("no --secret in RunArgs")
	}
	if !containsStr(args[firstSecretIdx], secret.MasterKeyName) {
		t.Errorf("first secret is %q, want master key", args[firstSecretIdx])
	}
}

func containsStr(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || (len(s) > len(sub) && func() bool {
		for i := 0; i <= len(s)-len(sub); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	}()))
}
