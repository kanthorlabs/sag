package dxh_test

import (
	"os"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/kanthorlabs/sag/e2e/devx/dxh"
)

func TestChildEnvAllowList(t *testing.T) {
	input := []string{
		"HOME=/home/user",
		"PATH=/usr/bin:/bin",
		"TMPDIR=/tmp",
		"USER=user",
		"LANG=en_US.UTF-8",
		"SAG_TEST_IMAGE=localhost/sag:devx-e2e",
		"CLAUDE_CONFIG_DIR=/should/not/pass",
		"PI_CODING_AGENT_DIR=/should/not/pass",
		"NODE_EXTRA_CA_CERTS=/should/not/pass",
		"SHELL=/bin/zsh",
		"CLAUDE_API_KEY=secret",
		"ZDOTDIR=/should/not/pass",
		"XDG_CONFIG_HOME=/should/not/pass",
	}
	env, err := dxh.ChildEnv(input, t.TempDir())
	if err != nil {
		t.Fatalf("ChildEnv: %v", err)
	}
	for _, e := range env {
		key, _, _ := strings.Cut(e, "=")
		if key == "NODE_EXTRA_CA_CERTS" || key == "SHELL" || key == "ZDOTDIR" || key == "XDG_CONFIG_HOME" {
			t.Errorf("forbidden variable passed through: %s", e)
		}
	}
	required := []string{"HOME", "TMPDIR", "USER", "LANG", "SAG_TEST_IMAGE"}
	for _, req := range required {
		if !slices.ContainsFunc(env, func(e string) bool { return strings.HasPrefix(e, req+"=") }) {
			t.Errorf("required variable %s missing from child env", req)
		}
	}
}

func TestChildEnvAWSKeysOnlyWithLocalStack(t *testing.T) {
	operator := []string{
		"HOME=/home/user",
		"AWS_ACCESS_KEY_ID=operator-key",
		"AWS_SECRET_ACCESS_KEY=operator-secret",
		"AWS_PROFILE=operator",
		"AWS_SESSION_TOKEN=operator-token",
	}
	env, err := dxh.ChildEnv(operator, t.TempDir())
	if err != nil {
		t.Fatalf("ChildEnv: %v", err)
	}
	for _, e := range env {
		if strings.HasPrefix(e, "AWS_") {
			t.Errorf("AWS variable passed without SAG_TEST_AWS_ENDPOINT: %s", e)
		}
	}

	env, err = dxh.ChildEnv(append(operator, "SAG_TEST_AWS_ENDPOINT=http://host.containers.internal:4566"), t.TempDir())
	if err != nil {
		t.Fatalf("ChildEnv: %v", err)
	}
	for _, want := range []string{"AWS_ACCESS_KEY_ID=operator-key", "AWS_SECRET_ACCESS_KEY=operator-secret"} {
		if !slices.Contains(env, want) {
			t.Errorf("%s missing with SAG_TEST_AWS_ENDPOINT set", want)
		}
	}
	for _, e := range env {
		if strings.HasPrefix(e, "AWS_PROFILE=") || strings.HasPrefix(e, "AWS_SESSION_TOKEN=") {
			t.Errorf("AWS variable outside the allow list passed: %s", e)
		}
	}
}

func TestChildEnvEmptyGroupDirErrors(t *testing.T) {
	_, err := dxh.ChildEnv(nil, "")
	if err == nil {
		t.Fatal("expected error for empty groupDir, got nil")
	}
}

func TestHarnessDirsAreTestDirs(t *testing.T) {
	input := []string{"HOME=/home/user"}
	env1, err := dxh.ChildEnv(input, t.TempDir())
	if err != nil {
		t.Fatalf("ChildEnv: %v", err)
	}
	env2, err := dxh.ChildEnv(input, t.TempDir())
	if err != nil {
		t.Fatalf("ChildEnv: %v", err)
	}

	get := func(env []string, key string) string {
		for _, e := range env {
			k, v, _ := strings.Cut(e, "=")
			if k == key {
				return v
			}
		}
		return ""
	}

	claude1 := get(env1, "CLAUDE_CONFIG_DIR")
	claude2 := get(env2, "CLAUDE_CONFIG_DIR")
	pi1 := get(env1, "PI_CODING_AGENT_DIR")
	pi2 := get(env2, "PI_CODING_AGENT_DIR")

	if claude1 == "" || pi1 == "" {
		t.Fatal("CLAUDE_CONFIG_DIR or PI_CODING_AGENT_DIR missing when groupDir given")
	}
	if claude1 == claude2 {
		t.Error("CLAUDE_CONFIG_DIR should differ between groups")
	}
	if pi1 == pi2 {
		t.Error("PI_CODING_AGENT_DIR should differ between groups")
	}
	if _, err := os.Stat(claude1); err != nil {
		t.Errorf("CLAUDE_CONFIG_DIR %q not created: %v", claude1, err)
	}
	if _, err := os.Stat(pi1); err != nil {
		t.Errorf("PI_CODING_AGENT_DIR %q not created: %v", pi1, err)
	}
}

func TestChildEnvXDGRuntimeDirOnLinuxOnly(t *testing.T) {
	out, err := dxh.ChildEnv([]string{"XDG_RUNTIME_DIR=/run/user/1000", "DBUS_SESSION_BUS_ADDRESS=unix:path=/x"}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	has := slices.Contains(out, "XDG_RUNTIME_DIR=/run/user/1000")
	if want := runtime.GOOS == "linux"; has != want {
		t.Errorf("XDG_RUNTIME_DIR passed = %v; want %v on %s", has, want, runtime.GOOS)
	}
	if slices.ContainsFunc(out, func(e string) bool { return strings.HasPrefix(e, "DBUS_SESSION_BUS_ADDRESS=") }) {
		t.Error("DBUS_SESSION_BUS_ADDRESS passed; want removed")
	}
}
