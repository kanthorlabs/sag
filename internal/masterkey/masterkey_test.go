package masterkey_test

import (
	"context"
	"io"
	"testing"

	"github.com/kanthorlabs/sag/internal/masterkey"
	"github.com/kanthorlabs/sag/internal/podman"
	"github.com/kanthorlabs/sag/internal/stepline"
)

type fake struct {
	calls []fakeCall
	fn    func(args []string, stdin []byte) ([]byte, error)
}

type fakeCall struct {
	args  []string
	stdin []byte
}

func (f *fake) Run(_ context.Context, stdin io.Reader, args ...string) ([]byte, error) {
	var buf []byte
	if stdin != nil {
		buf, _ = io.ReadAll(stdin)
	}
	f.calls = append(f.calls, fakeCall{args: args, stdin: buf})
	return f.fn(args, buf)
}

func TestValueOnStdinOnly(t *testing.T) {
	fr := &fake{fn: func(args []string, stdin []byte) ([]byte, error) {
		if len(args) >= 2 && args[0] == "secret" && args[1] == "inspect" {
			return nil, &podman.RunError{ExitCode: 1, Stderr: "no such secret"}
		}
		return nil, nil
	}}
	out := stepline.New(io.Discard)
	changed, err := masterkey.Ensure(context.Background(), fr, out)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("expected changed=true when secret is absent")
	}

	var createCall *fakeCall
	for i := range fr.calls {
		c := &fr.calls[i]
		if len(c.args) >= 2 && c.args[0] == "secret" && c.args[1] == "create" {
			createCall = c
		}
	}
	if createCall == nil {
		t.Fatal("no secret create call found")
	}
	if len(createCall.stdin) == 0 {
		t.Fatal("no value on stdin")
	}
	value := string(createCall.stdin)
	for _, arg := range createCall.args {
		if arg == value {
			t.Fatalf("key value found in args: %q", arg)
		}
	}
}

func TestNeverReplaced(t *testing.T) {
	fr := &fake{fn: func(args []string, stdin []byte) ([]byte, error) {
		return []byte(`[{"ID":"abc"}]`), nil
	}}
	out := stepline.New(io.Discard)
	changed, err := masterkey.Ensure(context.Background(), fr, out)
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Fatal("expected changed=false when secret exists")
	}
	for _, c := range fr.calls {
		if len(c.args) >= 2 && c.args[0] == "secret" && c.args[1] == "create" {
			t.Fatalf("unexpected secret create call: %v", c.args)
		}
	}
}

func TestPodmanFailureCreatesNothing(t *testing.T) {
	fr := &fake{fn: func(args []string, stdin []byte) ([]byte, error) {
		return nil, &podman.RunError{ExitCode: 125, Stderr: "Error: unable to connect to Podman socket"}
	}}
	changed, err := masterkey.Ensure(context.Background(), fr, stepline.New(io.Discard))
	if err == nil || changed {
		t.Fatalf("Ensure = %v, %v; want an error and no change", changed, err)
	}
	if len(fr.calls) != 1 {
		t.Fatalf("calls = %v; want only the inspect call", fr.calls)
	}
}
