//go:build linux

package tailscale

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/kanthorlabs/sag/internal/stepline"
)

const testTailscalePath = "/usr/bin/tailscale"

func TestOperatorPrompt(t *testing.T) {
	var buf bytes.Buffer
	out := stepline.New(&buf)

	cmd := &testCmd{
		path: testTailscalePath,
		responses: map[string]func() ([]byte, error){
			"debug prefs": func() ([]byte, error) {
				return []byte(`{"OperatorUser":"other"}`), nil
			},
			"sudo " + testTailscalePath + " set --operator=testuser": func() ([]byte, error) {
				return nil, nil
			},
		},
	}

	asked := false
	ask := func() (bool, error) {
		asked = true
		return true, nil
	}

	err := EnsureOperator(context.Background(), cmd, "testuser", true, ask, out)
	if err != nil {
		t.Fatalf("EnsureOperator: %v", err)
	}
	if !asked {
		t.Fatal("EnsureOperator did not call ask when operator not set and terminal available")
	}
	want := "sudo " + testTailscalePath + " set --operator=testuser"
	found := false
	for _, call := range cmd.recorded {
		if call == want {
			found = true
		}
	}
	if !found {
		t.Fatalf("EnsureOperator did not record %q; got: %v", want, cmd.recorded)
	}
}

func TestOperatorNoTerminalSkips(t *testing.T) {
	var buf bytes.Buffer
	out := stepline.New(&buf)

	cmd := &testCmd{
		path: testTailscalePath,
		responses: map[string]func() ([]byte, error){
			"debug prefs": func() ([]byte, error) {
				return []byte(`{"OperatorUser":"other"}`), nil
			},
		},
	}

	ask := func() (bool, error) {
		t.Fatal("ask should not be called when no terminal")
		return false, nil
	}

	err := EnsureOperator(context.Background(), cmd, "testuser", false, ask, out)
	if !errors.Is(err, ErrOperatorSkip) {
		t.Fatalf("EnsureOperator returned %v; want ErrOperatorSkip", err)
	}
	output := buf.String()
	if !bytes.Contains([]byte(output), []byte("skip tailscale operator not set; loopback only")) {
		t.Fatalf("EnsureOperator did not write skip line; got %q", output)
	}
	for _, call := range cmd.recorded {
		if call == "sudo "+testTailscalePath+" set --operator=testuser" {
			t.Fatal("RunWithSudo was called despite no terminal")
		}
	}
}

func TestOperatorSetBeforeServe(t *testing.T) {
	var buf bytes.Buffer
	out := stepline.New(&buf)

	sudoSetAlice := "sudo " + testTailscalePath + " set --operator=alice"
	cmd := &testCmd{
		path: testTailscalePath,
		responses: map[string]func() ([]byte, error){
			"debug prefs": func() ([]byte, error) {
				return []byte(`{"OperatorUser":"other"}`), nil
			},
			sudoSetAlice: func() ([]byte, error) {
				return nil, nil
			},
		},
	}

	err := EnsureOperator(context.Background(), cmd, "alice", true, func() (bool, error) {
		return true, nil
	}, out)
	if err != nil {
		t.Fatalf("EnsureOperator: %v", err)
	}

	if len(cmd.recorded) < 2 {
		t.Fatalf("expected at least 2 recorded calls; got %v", cmd.recorded)
	}
	if cmd.recorded[0] != "debug prefs" {
		t.Fatalf("first call = %q; want %q", cmd.recorded[0], "debug prefs")
	}
	setIdx := -1
	for i, call := range cmd.recorded {
		if call == sudoSetAlice {
			setIdx = i
		}
	}
	if setIdx < 0 {
		t.Fatalf("%q not in recorded calls: %v", sudoSetAlice, cmd.recorded)
	}
	if setIdx < 1 {
		t.Fatalf("sudo set called before debug prefs; recorded: %v", cmd.recorded)
	}
}

func TestOperatorAlreadySet(t *testing.T) {
	var buf bytes.Buffer
	out := stepline.New(&buf)

	cmd := &testCmd{
		path: testTailscalePath,
		responses: map[string]func() ([]byte, error){
			"debug prefs": func() ([]byte, error) {
				return []byte(`{"OperatorUser":"alice"}`), nil
			},
		},
	}

	ask := func() (bool, error) {
		t.Fatal("ask should not be called when operator already set")
		return false, nil
	}

	err := EnsureOperator(context.Background(), cmd, "alice", true, ask, out)
	if err != nil {
		t.Fatalf("EnsureOperator: %v", err)
	}
	for _, call := range cmd.recorded {
		if call == "sudo "+testTailscalePath+" set --operator=alice" {
			t.Fatal("RunWithSudo was called when operator already set")
		}
	}
}
