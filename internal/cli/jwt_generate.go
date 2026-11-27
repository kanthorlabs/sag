package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"time"

	"github.com/kanthorlabs/sag/internal/config"
	"github.com/kanthorlabs/sag/internal/keys"
	"github.com/kanthorlabs/sag/internal/secret"
	"github.com/kanthorlabs/sag/internal/token"
)

const jwtGenerateUsage = "usage: sag jwt generate <sub> [--name <display>]"

var errNoTerminal = errors.New("standard input and standard output must be terminals")

type jwtIssuer struct {
	Lifetime time.Duration
	Known    func(sub string) bool
	Key      func() ([]byte, error)
}

var loadJWTIssuer = jwtIssuerFromPaths

func jwtIssuerFromPaths() (jwtIssuer, error) {
	cfg, err := config.Load(paths.Config)
	if err != nil {
		return jwtIssuer{}, err
	}
	return jwtIssuer{
		Lifetime: cfg.Auth.TokenLifetime,
		Known:    cfg.HasAgent,
		Key: func() ([]byte, error) {
			creds, err := secret.Load(paths.Secrets, paths.SecretUID, []string{secret.MasterKeyName})
			if err != nil {
				return nil, err
			}
			master, err := secret.DecodeMasterKey(creds[secret.MasterKeyName])
			if err != nil {
				return nil, err
			}
			return keys.JWTKey(master, cfg.Auth.Version)
		},
	}, nil
}

func jwtGenerate(args []string, std IO) int {
	if std.Mode == ModeHost {
		return hostJWTGenerate(args, std)
	}
	if !std.IsTerminal(std.In) || !std.IsTerminal(std.Out) {
		return fail(std, errNoTerminal)
	}
	sub, name, ok := parseJWTGenerateArgs(args)
	if !ok {
		fmt.Fprintln(std.Err, jwtGenerateUsage)
		return ExitUsage
	}
	issuer, err := loadJWTIssuer()
	if err != nil {
		return fail(std, err)
	}
	if !issuer.Known(sub) {
		return fail(std, fmt.Errorf("sub %q is not in the agents list of any resource", sub))
	}
	key, err := issuer.Key()
	if err != nil {
		return fail(std, err)
	}
	raw, err := token.Issue(key, sub, name, time.Now(), issuer.Lifetime)
	if err != nil {
		return fail(std, err)
	}
	fmt.Fprintln(std.Out, raw)
	return ExitOK
}

func parseJWTGenerateArgs(args []string) (string, string, bool) {
	if len(args) == 0 {
		return "", "", false
	}
	sub := args[0]
	fs := flag.NewFlagSet("jwt generate", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	name := fs.String("name", sub, "")
	if err := fs.Parse(args[1:]); err != nil || fs.NArg() != 0 {
		return "", "", false
	}
	return sub, *name, true
}
