package cli

import (
	"fmt"
	"io"
	"os"
	"slices"

	"github.com/kanthorlabs/sag/internal/config"
	"github.com/kanthorlabs/sag/internal/daemon"
	"github.com/kanthorlabs/sag/internal/secret"
	"github.com/kanthorlabs/sag/internal/sql"
)

var Version = "dev"

const (
	ExitOK      = 0
	ExitFailure = 1
	ExitUsage   = 2
	ExitRefusal = 3
)

const usage = `usage:
  sag bootstrap [--auto-approve <name>@<version>]... [--autostart yes|no]
  sag service start
  sag service stop
  sag credential create <name> --kind <kind> [--agent <sub>]...
  sag credential list [--json]
  sag credential inspect <name> [--json]
  sag credential delete <name>
  sag logs daemon [--follow] [--since <duration>] [--tail <n>]
  sag logs audit [--date <YYYY-MM-DD>]
  sag agent setup <claude-code|pi>
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
	Mode       Mode
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
	kind commandKind
}

var commands = []command{
	{path: []string{"bootstrap"}, run: bootstrapCmd, kind: kindHostOnly},
	{path: []string{"service", "start"}, run: serviceStart, kind: kindHostOnly},
	{path: []string{"service", "stop"}, run: serviceStop, kind: kindHostOnly},
	{path: []string{"credential", "create"}, run: credentialCreate, kind: kindHostOnly},
	{path: []string{"credential", "list"}, run: credentialList, kind: kindHostOnly},
	{path: []string{"credential", "inspect"}, run: credentialInspect, kind: kindHostOnly},
	{path: []string{"credential", "delete"}, run: credentialDelete, kind: kindHostOnly},
	{path: []string{"logs", "daemon"}, run: logsDaemon, kind: kindHostOnly},
	{path: []string{"logs", "audit"}, run: logsAudit, kind: kindHostOnly},
	{path: []string{"agent", "setup"}, run: agentSetup, kind: kindHostOnly},
	{path: []string{"serve"}, run: serveProduct, kind: kindInternal},
	{path: []string{"config", "check"}, run: configCheck, kind: kindAny},
	{path: []string{"resource", "test"}, run: resourceTest, kind: kindHostOnly},
	{path: []string{"jwt", "generate"}, run: jwtGenerate, kind: kindHostOnly},
	{path: []string{"extension", "approve"}, run: extensionApprove, kind: kindHostOnly},
	{path: []string{"extension", "revoke"}, run: extensionRevoke, kind: kindHostOnly},
	{path: []string{"version"}, run: version, kind: kindAny},
	{path: []string{"credential", "check"}, run: credentialCheck, kind: kindInternal},
	{path: []string{"audit", "read"}, run: auditRead, kind: kindInternal},
	{path: []string{"agent", "issue"}, run: agentIssue, kind: kindInternal},
}

func Run(args []string, std IO) int {
	return RunWith(args, std, nil)
}

func RunWith(args []string, std IO, wrap func(sql.Dialect) sql.Dialect) int {
	if err := daemon.ApplyProcessRules(); err != nil {
		return fail(std, err)
	}
	std.Mode = resolveMode(std.Mode)
	if wrap != nil && len(args) > 0 && args[0] == "serve" {
		return serve(args[1:], std, wrap)
	}
	for _, c := range commands {
		if len(args) >= len(c.path) && slices.Equal(args[:len(c.path)], c.path) {
			if code, refused := commandGuard(std.Mode, c, std); refused {
				return code
			}
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
