package cli

import (
	"fmt"
	"time"

	"github.com/kanthorlabs/sag/internal/token"
)

func agentIssue(args []string, std IO) int {
	if std.IsTerminal(std.Out) {
		return ExitRefusal
	}
	if len(args) != 1 {
		fmt.Fprintln(std.Err, "usage: sag agent issue <sub>")
		return ExitUsage
	}
	sub := args[0]
	issuer, err := loadAgentIssuer()
	if err != nil {
		return fail(std, err)
	}
	key, err := issuer.Key()
	if err != nil {
		return fail(std, err)
	}
	raw, err := token.Issue(key, sub, sub, time.Now(), issuer.Lifetime)
	if err != nil {
		return fail(std, err)
	}
	fmt.Fprintln(std.Out, raw)
	return ExitOK
}

var loadAgentIssuer = jwtIssuerFromPaths
