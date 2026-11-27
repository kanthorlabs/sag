package agentsetup

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func caTrustConflictFixture(t *testing.T) (Options, string) {
	t.Helper()
	dir := t.TempDir()
	cfg := filepath.Join(dir, "cfg")
	claudeDir := filepath.Join(dir, "claude")
	binDir := filepath.Join(dir, "bin")
	for _, d := range []string{cfg, claudeDir, binDir} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	caPath := filepath.Join(cfg, "ca.pem")
	otherPath := filepath.Join(dir, "other.pem")
	os.WriteFile(caPath, generateTestCert(t), 0o600)
	os.WriteFile(otherPath, generateTestCert(t), 0o600)
	os.WriteFile(filepath.Join(binDir, "claude"), []byte("#!/bin/sh\n"), 0o700)
	settings := filepath.Join(claudeDir, "settings.json")
	os.WriteFile(settings, []byte(`{"env":{"NODE_EXTRA_CA_CERTS":"`+otherPath+`"}}`), 0o600)
	opts := Options{
		Harness:     HarnessClaudeCode,
		Sub:         "claude-code",
		URL:         "https://localhost:8443/mcp",
		CfgDir:      cfg,
		Home:        dir,
		GOOS:        "darwin",
		DenyRules:   "no",
		UnderLock:   true,
		AnswersPath: filepath.Join(cfg, "answers.json"),
		Env:         map[string]string{"PATH": binDir, "CLAUDE_CONFIG_DIR": claudeDir},
	}
	bundle := filepath.Join(cfg, "ca-bundle.pem")
	want := fmt.Sprintf("fail ca-trust NODE_EXTRA_CA_CERTS is %s; run: cat %q %q > %q; then set NODE_EXTRA_CA_CERTS to %s in %s",
		otherPath, otherPath, caPath, bundle, bundle, settings)
	return opts, want
}

func TestB105PlanPrintsCATrustLineOnce(t *testing.T) {
	opts, want := caTrustConflictFixture(t)
	var buf bytes.Buffer
	prev := agentOut
	SetOut(&buf)
	t.Cleanup(func() { SetOut(prev) })

	_, err := PlanOnly(context.Background(), opts)
	if err == nil {
		t.Fatal("PlanOnly: want a CA trust conflict")
	}
	found := false
	for _, line := range strings.Split(buf.String(), "\n") {
		if line == want {
			found = true
		}
	}
	if !found {
		t.Errorf("output has no line %q; got %q", want, buf.String())
	}
	var caErr *CATrustConflictError
	if !errors.As(err, &caErr) {
		t.Errorf("errors.As(%T, *CATrustConflictError) = false; want true", err)
	}
}

func TestB90FishForeignEnvIsConflict(t *testing.T) {
	dir := t.TempDir()
	caPath := filepath.Join(dir, "ca.pem")
	otherPath := filepath.Join(dir, "other.pem")
	os.WriteFile(caPath, generateTestCert(t), 0o600)
	os.WriteFile(otherPath, generateTestCert(t), 0o600)
	opts := Options{
		Harness: HarnessPi,
		CfgDir:  dir,
		GOOS:    "darwin",
		Home:    dir,
		Env:     map[string]string{"SHELL": "/bin/fish", "NODE_EXTRA_CA_CERTS": otherPath},
	}
	e := caTrustConflict(opts, PiPaths("", dir))
	if e == nil || e.Source != "the environment" {
		t.Fatalf("caTrustConflict = %v; want a conflict with source the environment", e)
	}
}

func TestDeferCATrustPlanHasNoCATrustConflict(t *testing.T) {
	opts, _ := caTrustConflictFixture(t)
	opts.DeferCATrust = true
	var buf bytes.Buffer
	prev := agentOut
	SetOut(&buf)
	t.Cleanup(func() { SetOut(prev) })

	if _, err := PlanOnly(context.Background(), opts); err != nil {
		t.Fatalf("PlanOnly with DeferCATrust: %v; output %q", err, buf.String())
	}
	if strings.Contains(buf.String(), "ca-trust") {
		t.Errorf("output has a ca-trust line with DeferCATrust; got %q", buf.String())
	}
}
