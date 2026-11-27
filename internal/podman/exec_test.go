package podman

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExecBuildsArgsNoStdin(t *testing.T) {
	f := &fakeExec{}
	er := execRunner{exec: f.run}
	_, _ = er.Exec(context.Background(), nil, []string{"sag", "version"})
	if len(f.calls) != 1 {
		t.Fatalf("want 1 call, got %d", len(f.calls))
	}
	args := f.calls[0].args
	want := []string{"exec", "sag", "version"}
	if !equalSlice(args, want) {
		t.Errorf("args = %v, want %v", args, want)
	}
	for _, a := range args {
		if a == "-i" {
			t.Errorf("unexpected -i in args when stdin is nil: %v", args)
		}
	}
}

func TestExecBuildsArgsWithStdin(t *testing.T) {
	f := &fakeExec{}
	er := execRunner{exec: f.run}
	_, _ = er.Exec(context.Background(), strings.NewReader("data"), []string{"sag", "cred", "check"})
	if len(f.calls) != 1 {
		t.Fatalf("want 1 call, got %d", len(f.calls))
	}
	args := f.calls[0].args
	want := []string{"exec", "-i", "sag", "cred", "check"}
	if !equalSlice(args, want) {
		t.Errorf("args = %v, want %v", args, want)
	}
}

func TestExecTTYBuildsArgs(t *testing.T) {
	var gotArgs []string
	er := execRunner{ttyExec: func(_ context.Context, args []string) error {
		gotArgs = args
		return nil
	}}
	err := er.ExecTTY(context.Background(), []string{"sag", "jwt", "generate", "sub"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"exec", "-i", "-t", "sag", "jwt", "generate", "sub"}
	if !equalSlice(gotArgs, want) {
		t.Errorf("args = %v, want %v", gotArgs, want)
	}
}

func TestExecTTYPropagatesError(t *testing.T) {
	er := execRunner{ttyExec: func(_ context.Context, _ []string) error {
		return &RunError{ExitCode: 3, Stderr: "refused"}
	}}
	err := er.ExecTTY(context.Background(), []string{"sag", "ext", "approve", "res"})
	var re *RunError
	if !errors.As(err, &re) {
		t.Fatalf("want *RunError, got %T: %v", err, err)
	}
	if re.ExitCode != 3 {
		t.Errorf("ExitCode = %d, want 3", re.ExitCode)
	}
}

func TestRunnerExecTTYExitCode(t *testing.T) {
	dir := t.TempDir()
	stub := filepath.Join(dir, "podman-stub")
	if err := os.WriteFile(stub, []byte("#!/bin/sh\nexit 3"), 0o755); err != nil {
		t.Fatal(err)
	}
	r := Runner{Path: stub}
	err := r.ExecTTY(context.Background(), "sag", "jwt", "generate", "sub")
	var re *RunError
	if !errors.As(err, &re) {
		t.Fatalf("want *RunError, got %T: %v", err, err)
	}
	if re.ExitCode != 3 {
		t.Errorf("ExitCode = %d, want 3", re.ExitCode)
	}
}

func TestRunnerExecPassesStdin(t *testing.T) {
	dir := t.TempDir()
	stub := filepath.Join(dir, "podman-stub")
	if err := os.WriteFile(stub, []byte("#!/bin/sh\ncat\nexit 0"), 0o755); err != nil {
		t.Fatal(err)
	}
	r := Runner{Path: stub}
	out, err := r.Exec(context.Background(), strings.NewReader("hello-stdin"), "sag", "audit", "read")
	if err != nil {
		t.Fatalf("Exec returned error: %v", err)
	}
	if !strings.Contains(string(out), "hello-stdin") {
		t.Errorf("output = %q, want to contain hello-stdin", string(out))
	}
}

func equalSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range b {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
