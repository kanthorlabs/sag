package cli

import (
	"bytes"
	"strings"
	"testing"
)

func testIO() (IO, *bytes.Buffer) {
	errOut := &bytes.Buffer{}
	return IO{
		In:         strings.NewReader(""),
		Out:        &bytes.Buffer{},
		Err:        errOut,
		IsTerminal: func(any) bool { return false },
	}, errOut
}

func TestRunWithoutCommandPrintsUsage(t *testing.T) {
	std, errOut := testIO()
	if code := Run(nil, std); code != ExitUsage {
		t.Fatalf("Run(nil) = %d; want %d", code, ExitUsage)
	}
	if !strings.Contains(errOut.String(), "usage") {
		t.Fatalf("stderr = %q; want usage text", errOut.String())
	}
}

func TestRunUnknownCommandPrintsUsage(t *testing.T) {
	for _, args := range [][]string{{"nope"}, {"jwt"}, {"extension", "delete"}} {
		std, _ := testIO()
		if code := Run(args, std); code != ExitUsage {
			t.Fatalf("Run(%q) = %d; want %d", args, code, ExitUsage)
		}
	}
}

func TestRunDispatchesEachCommand(t *testing.T) {
	var got [][]string
	saved := commands
	defer func() { commands = saved }()
	commands = nil
	for _, c := range saved {
		path := c.path
		commands = append(commands, command{path: path, run: func(args []string, std IO) int {
			got = append(got, append(append([]string{}, path...), args...))
			return ExitOK
		}})
	}
	calls := [][]string{
		{"serve"},
		{"config", "check", "--config", "x.yaml"},
		{"resource", "test", "orders"},
		{"jwt", "generate", "agent-a", "--name", "A"},
		{"extension", "approve", "orders"},
		{"extension", "revoke", "orders"},
		{"version"},
	}
	for _, args := range calls {
		std, _ := testIO()
		if code := Run(args, std); code != ExitOK {
			t.Fatalf("Run(%q) = %d; want %d", args, code, ExitOK)
		}
	}
	if len(got) != len(calls) {
		t.Fatalf("dispatched %d commands; want %d", len(got), len(calls))
	}
	for i := range calls {
		if strings.Join(got[i], " ") != strings.Join(calls[i], " ") {
			t.Fatalf("call %d dispatched %q; want %q", i, got[i], calls[i])
		}
	}
}
