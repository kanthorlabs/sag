package deploy_test

import (
	"slices"
	"testing"

	"github.com/kanthorlabs/sag/internal/config"
	"github.com/kanthorlabs/sag/internal/deploy"
)

func minCfg(t *testing.T) *config.Config {
	t.Helper()
	cfg, err := config.Load("../../deploy/dev/sag.yaml")
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestRunArgsLogFlags(t *testing.T) {
	args := deploy.RunArgs(minCfg(t), deploy.RunOpts{Image: "img", ConfigSource: "/etc/sag/sag.yaml"})
	joined := slices.Contains(args, "--log-driver")
	if !joined {
		t.Error("RunArgs has no --log-driver")
	}
	for i, a := range args {
		if a == "--log-driver" {
			if i+1 >= len(args) || args[i+1] != "k8s-file" {
				t.Errorf("--log-driver value = %q; want k8s-file", args[i+1])
			}
			break
		}
	}
	if !slices.Contains(args, "--log-opt") {
		t.Error("RunArgs has no --log-opt")
	}
	for i, a := range args {
		if a == "--log-opt" {
			if i+1 >= len(args) || args[i+1] != "max-size=10mb" {
				t.Errorf("--log-opt value = %q; want max-size=10mb", args[i+1])
			}
			break
		}
	}
}

func TestRunArgsPublishLoopbackOnly(t *testing.T) {
	args := deploy.RunArgs(minCfg(t), deploy.RunOpts{Image: "img", ConfigSource: "/etc/sag/sag.yaml"})
	for i, a := range args {
		if a == "-p" {
			if i+1 >= len(args) {
				t.Fatal("-p has no value")
			}
			got := args[i+1]
			if got != "127.0.0.1:8443:8443" {
				t.Errorf("-p %q; want 127.0.0.1:8443:8443", got)
			}
			return
		}
	}
	t.Error("RunArgs has no -p flag")
}
