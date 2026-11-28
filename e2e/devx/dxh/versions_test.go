package dxh_test

import (
	"context"
	"regexp"
	"testing"

	"github.com/kanthorlabs/sag/e2e/devx/dxh"
)

var ipRE = regexp.MustCompile(`\d+\.\d+\.\d+\.\d+`)

func TestEnvHasNoIP(t *testing.T) {
	f := newFake()
	f.set("sw_vers -productVersion", []byte("15.3.1\n"), nil)
	f.set("podman --version", []byte("podman version 5.5.1\n"), nil)
	f.set("claude --version", []byte("2.1.332\n"), nil)
	f.set("pi --version", []byte("pi 1.6.1\n"), nil)
	f.set("tailscale version", []byte("1.76.6\n"), nil)

	env, err := dxh.RecordWith(context.Background(), f)
	if err != nil {
		t.Fatalf("RecordWith: %v", err)
	}

	for field, val := range map[string]string{
		"OS":         env.OS,
		"Podman":     env.Podman,
		"ClaudeCode": env.ClaudeCode,
		"Pi":         env.Pi,
		"Tailscale":  env.Tailscale,
	} {
		if ipRE.MatchString(val) {
			t.Errorf("Env.%s contains IP address: %q", field, val)
		}
	}
}
