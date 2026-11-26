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
		Mode:       ModeContainer,
	}, errOut
}

func hostIO() (IO, *bytes.Buffer) {
	errOut := &bytes.Buffer{}
	return IO{
		In:         strings.NewReader(""),
		Out:        &bytes.Buffer{},
		Err:        errOut,
		IsTerminal: func(any) bool { return false },
		Mode:       ModeHost,
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

func TestUID0Refused(t *testing.T) {
	saved := uid
	uid = func() int { return 0 }
	defer func() { uid = saved }()

	for _, args := range [][]string{
		{"bootstrap"},
		{"service", "start"},
		{"service", "stop"},
		{"credential", "create"},
		{"jwt", "generate", "a"},
	} {
		std, errOut := hostIO()
		if code := Run(args, std); code != ExitFailure {
			t.Fatalf("Run(%q) as root = %d; want %d", args, code, ExitFailure)
		}
		if !strings.Contains(errOut.String(), "root") {
			t.Fatalf("Run(%q) as root stderr = %q; want root message", args, errOut.String())
		}
	}
}

func TestInternalCommandOnHostExit2(t *testing.T) {
	for _, args := range [][]string{
		{"serve"},
		{"credential", "check"},
		{"audit", "read"},
		{"agent", "issue", "a"},
	} {
		std, errOut := hostIO()
		if code := Run(args, std); code != ExitUsage {
			t.Fatalf("Run(%q) on host = %d; want %d (stderr: %q)", args, code, ExitUsage, errOut.String())
		}
		if !strings.Contains(errOut.String(), "container") {
			t.Fatalf("Run(%q) on host stderr = %q; want container message", args, errOut.String())
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
