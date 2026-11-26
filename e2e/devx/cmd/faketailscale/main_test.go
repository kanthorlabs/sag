package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIsServeRule(t *testing.T) {
	cases := []struct {
		args  []string
		match bool
	}{
		{[]string{"--bg", "--tcp", "8443", "tcp://127.0.0.1:8443"}, true},
		{[]string{"--bg", "--tcp", "8443", "tcp://127.0.0.1:9090"}, false},
		{[]string{"--bg", "--tcp", "8443"}, false},
		{[]string{"status", "--json"}, false},
	}
	for _, tc := range cases {
		got := isServeRule(tc.args)
		if got != tc.match {
			t.Errorf("isServeRule(%v) = %v; want %v", tc.args, got, tc.match)
		}
	}
}

func TestHandleIPValid(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ts_ip4"), []byte("100.64.1.1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	code := dispatch(&stdout, &bytes.Buffer{}, dir, []string{"ip", "-4"})
	if code != 0 {
		t.Fatalf("exit %d; want 0", code)
	}
	if strings.TrimSpace(stdout.String()) != "100.64.1.1" {
		t.Fatalf("output = %q; want 100.64.1.1", stdout.String())
	}
}

func TestHandleIPDown(t *testing.T) {
	dir := t.TempDir()
	code := dispatch(&bytes.Buffer{}, &bytes.Buffer{}, dir, []string{"ip", "-4"})
	if code != 1 {
		t.Fatalf("exit %d; want 1", code)
	}
}

func TestHandleIPExitMarker(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ts_ip4"), []byte("exit:1"), 0o600); err != nil {
		t.Fatal(err)
	}
	code := dispatch(&bytes.Buffer{}, &bytes.Buffer{}, dir, []string{"ip", "-4"})
	if code != 1 {
		t.Fatalf("exit %d; want 1", code)
	}
}

func TestHandleServeStatusEmpty(t *testing.T) {
	dir := t.TempDir()
	var stdout bytes.Buffer
	code := dispatch(&stdout, &bytes.Buffer{}, dir, []string{"serve", "status", "--json"})
	if code != 0 {
		t.Fatalf("exit %d; want 0", code)
	}
	if !strings.Contains(stdout.String(), "{}") {
		t.Fatalf("expected empty JSON object, got %q", stdout.String())
	}
}

func TestHandleServeApply(t *testing.T) {
	dir := t.TempDir()
	code := dispatch(&bytes.Buffer{}, &bytes.Buffer{}, dir, []string{"serve", "--bg", "--tcp", "8443", "tcp://127.0.0.1:8443"})
	if code != 0 {
		t.Fatalf("exit %d; want 0", code)
	}
	if _, err := os.Stat(filepath.Join(dir, "ts_rule")); err != nil {
		t.Fatal("ts_rule not written")
	}
}

func TestHandleServeStatusAfterApply(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ts_rule"), []byte("served\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	code := dispatch(&stdout, &bytes.Buffer{}, dir, []string{"serve", "status", "--json"})
	if code != 0 {
		t.Fatalf("exit %d; want 0", code)
	}
	out := stdout.String()
	if !strings.Contains(out, "127.0.0.1:8443") {
		t.Fatalf("serve status after apply missing SAG rule; got %q", out)
	}
}

func TestRecordArgs(t *testing.T) {
	dir := t.TempDir()
	recordArgs(dir, []string{"ip", "-4"})
	recordArgs(dir, []string{"serve", "status", "--json"})
	data, err := os.ReadFile(filepath.Join(dir, "ts_args"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 recorded lines; got %d: %q", len(lines), string(data))
	}
	if lines[0] != "ip -4" {
		t.Fatalf("first line = %q; want \"ip -4\"", lines[0])
	}
}
