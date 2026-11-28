//go:build linux

package tailscale

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/kanthorlabs/sag/internal/stepline"
)

func TestOperatorPrompt(t *testing.T) {
	var buf bytes.Buffer
	out := stepline.New(&buf)

	cmd := &testCmd{responses: map[string]func() ([]byte, error){
		"debug prefs": func() ([]byte, error) {
			return []byte(`{"OperatorUser":"other"}`), nil
		},
		"set --operator=testuser": func() ([]byte, error) {
			return nil, nil
		},
	}}

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
	found := false
	for _, call := range cmd.recorded {
		if call == "set --operator=testuser" {
			found = true
		}
	}
	if !found {
		t.Fatalf("EnsureOperator did not call set --operator=testuser; recorded: %v", cmd.recorded)
	}
}

func TestOperatorNoTerminalSkips(t *testing.T) {
	var buf bytes.Buffer
	out := stepline.New(&buf)

	cmd := &testCmd{responses: map[string]func() ([]byte, error){
		"debug prefs": func() ([]byte, error) {
			return []byte(`{"OperatorUser":"other"}`), nil
		},
	}}

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
}

func TestOperatorSetBeforeServe(t *testing.T) {
	var buf bytes.Buffer
	out := stepline.New(&buf)

	cmd := &testCmd{responses: map[string]func() ([]byte, error){
		"debug prefs": func() ([]byte, error) {
			return []byte(`{"OperatorUser":"other"}`), nil
		},
		"set --operator=alice": func() ([]byte, error) {
			return nil, nil
		},
	}}

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
		if call == "set --operator=alice" {
			setIdx = i
		}
	}
	if setIdx < 0 {
		t.Fatalf("set --operator=alice not in recorded calls: %v", cmd.recorded)
	}
}

func TestOperatorAlreadySet(t *testing.T) {
	var buf bytes.Buffer
	out := stepline.New(&buf)

	cmd := &testCmd{responses: map[string]func() ([]byte, error){
		"debug prefs": func() ([]byte, error) {
			return []byte(`{"OperatorUser":"alice"}`), nil
		},
	}}

	ask := func() (bool, error) {
		t.Fatal("ask should not be called when operator already set")
		return false, nil
	}

	err := EnsureOperator(context.Background(), cmd, "alice", true, ask, out)
	if err != nil {
		t.Fatalf("EnsureOperator: %v", err)
	}
	for _, call := range cmd.recorded {
		if call == "set --operator=alice" {
			t.Fatal("set was called when operator already set")
		}
	}
}
