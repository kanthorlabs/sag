package cli

import (
	"fmt"
	"io"
	"os"
	"slices"

	"github.com/kanthorlabs/sag/internal/config"
	"github.com/kanthorlabs/sag/internal/secret"
)

var Version = "dev"

const (
	ExitOK      = 0
	ExitFailure = 1
	ExitUsage   = 2
)

const usage = `usage:
  sag serve
  sag config check [--config <path>]
  sag resource test <name>
  sag jwt generate <sub> [--name <display>]
  sag extension approve <resource>
  sag extension revoke <resource>
  sag version`

type IO struct {
	In         io.Reader
	Out        io.Writer
	Err        io.Writer
	IsTerminal func(stream any) bool
}

func StdIO() IO {
	return IO{In: os.Stdin, Out: os.Stdout, Err: os.Stderr, IsTerminal: isTerminal}
}

type Paths struct {
	Config    string
	Secrets   string
	SecretUID int
	State     string
}

var paths = Paths{
	Config:    config.DefaultPath,
	Secrets:   secret.Dir,
	SecretUID: secret.ContainerUID,
	State:     "/var/lib/sag",
}

type command struct {
	path []string
	run  func(args []string, std IO) int
}

var commands = []command{
	{path: []string{"serve"}, run: serve},
	{path: []string{"config", "check"}, run: configCheck},
	{path: []string{"resource", "test"}, run: resourceTest},
	{path: []string{"jwt", "generate"}, run: jwtGenerate},
	{path: []string{"extension", "approve"}, run: extensionApprove},
	{path: []string{"extension", "revoke"}, run: extensionRevoke},
	{path: []string{"version"}, run: version},
}

func Run(args []string, std IO) int {
	for _, c := range commands {
		if len(args) >= len(c.path) && slices.Equal(args[:len(c.path)], c.path) {
			return c.run(args[len(c.path):], std)
		}
	}
	fmt.Fprintln(std.Err, usage)
	return ExitUsage
}

func fail(std IO, err error) int {
	fmt.Fprintf(std.Err, "sag: %v\n", err)
	return ExitFailure
}
