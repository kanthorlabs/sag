package dxh_test

import (
	"os"
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
	env := dxh.ChildEnv(input, "")
	for _, e := range env {
		key, _, _ := strings.Cut(e, "=")
		if key == "CLAUDE_CONFIG_DIR" || key == "PI_CODING_AGENT_DIR" || key == "NODE_EXTRA_CA_CERTS" || key == "SHELL" || strings.HasPrefix(key, "CLAUDE") || key == "ZDOTDIR" || key == "XDG_CONFIG_HOME" {
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

func TestHarnessDirsAreTestDirs(t *testing.T) {
	input := []string{"HOME=/home/user"}
	env1 := dxh.ChildEnv(input, t.TempDir())
	env2 := dxh.ChildEnv(input, t.TempDir())

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
