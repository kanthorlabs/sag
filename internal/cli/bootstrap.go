package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/kanthorlabs/sag/internal/bootstrap"
	"github.com/kanthorlabs/sag/internal/deps"
	"github.com/kanthorlabs/sag/internal/prompt"
	"github.com/kanthorlabs/sag/internal/stepline"
)

var errNotImplemented = errors.New("not implemented")

var runBootstrap = func(ctx context.Context, flags bootstrap.Flags, std IO) error {
	asker := prompt.NewAsker()
	out := stepline.New(std.Out)
	deps := bootstrap.DefaultDeps(asker, out, std.Out)
	return bootstrap.Run(ctx, flags, deps)
}

func bootstrapCmd(args []string, std IO) int {
	flags, ok := parseBootstrapFlags(args, std)
	if !ok {
		return ExitUsage
	}
	err := runBootstrap(context.Background(), flags, std)
	if err == nil {
		return ExitOK
	}
	var refusal *prompt.RefusalError
	if errors.As(err, &refusal) {
		fmt.Fprintf(std.Err, "sag: %v\n", err)
		return ExitRefusal
	}
	var installRefusal *deps.InstallRefusal
	if errors.As(err, &installRefusal) {
		fmt.Fprintf(std.Err, "sag: %v\n", err)
		return ExitRefusal
	}
	var sudoRefusal *deps.SudoRefusal
	if errors.As(err, &sudoRefusal) {
		fmt.Fprintf(std.Err, "sag: %v\n", err)
		return ExitRefusal
	}
	return fail(std, err)
}

func parseBootstrapFlags(args []string, std IO) (bootstrap.Flags, bool) {
	fs := flag.NewFlagSet("bootstrap", flag.ContinueOnError)
	fs.SetOutput(std.Err)

	var flags bootstrap.Flags
	flags.AgentSetup = make(map[string]string)

	var autoApprove multiFlag
	fs.Var(&autoApprove, "auto-approve", "approve install of <name>@<version>")

	var autostart string
	fs.StringVar(&autostart, "autostart", "", "yes|no")

	var agentSetup multiFlag
	fs.Var(&agentSetup, "agent-setup", "<harness>=yes|no")

	var denyRules string
	fs.StringVar(&denyRules, "deny-rules", "", "yes|no")

	var shellProfile string
	fs.StringVar(&shellProfile, "shell-profile", "", "yes|no")

	var guard string
	fs.StringVar(&guard, "guard", "", "yes|no")

	if err := fs.Parse(args); err != nil {
		return bootstrap.Flags{}, false
	}

	if fs.NArg() != 0 {
		fmt.Fprintf(std.Err, "sag: bootstrap: unexpected arguments: %v\n", fs.Args())
		return bootstrap.Flags{}, false
	}

	if !validYesNoFlag("--autostart", autostart, std.Err) {
		return bootstrap.Flags{}, false
	}
	if !validYesNoFlag("--deny-rules", denyRules, std.Err) {
		return bootstrap.Flags{}, false
	}
	if !validYesNoFlag("--shell-profile", shellProfile, std.Err) {
		return bootstrap.Flags{}, false
	}
	if !validYesNoFlag("--guard", guard, std.Err) {
		return bootstrap.Flags{}, false
	}

	for _, kv := range agentSetup {
		k, v, ok := strings.Cut(kv, "=")
		if !ok || (v != "yes" && v != "no") {
			fmt.Fprintf(std.Err, "sag: --agent-setup: want <harness>=yes|no, got %q\n", kv)
			return bootstrap.Flags{}, false
		}
		flags.AgentSetup[k] = v
	}

	flags.AutoApprove = autoApprove
	flags.Autostart = autostart
	flags.DenyRules = denyRules
	flags.ShellProfile = shellProfile
	flags.Guard = guard

	return flags, true
}

func validYesNoFlag(name, value string, w io.Writer) bool {
	if value == "" || value == "yes" || value == "no" {
		return true
	}
	fmt.Fprintf(w, "sag: %s: want yes|no, got %q\n", name, value)
	return false
}

type multiFlag []string

func (m *multiFlag) String() string {
	return strings.Join(*m, ",")
}

func (m *multiFlag) Set(v string) error {
	*m = append(*m, v)
	return nil
}
