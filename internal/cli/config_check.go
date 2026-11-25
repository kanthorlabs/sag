package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"

	"github.com/kanthorlabs/sag/internal/config"
	"github.com/kanthorlabs/sag/internal/core"
	"github.com/kanthorlabs/sag/internal/secret"
)

const configCheckUsage = "usage: sag config check [--config <path>]"

func configCheck(args []string, std IO) int {
	flags := flag.NewFlagSet("config check", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	path := flags.String("config", paths.Config, "")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 {
		fmt.Fprintln(std.Err, configCheckUsage)
		return ExitUsage
	}
	cfg, err := config.Load(*path)
	if err != nil {
		return fail(std, err)
	}
	if err := checkKinds(cfg); err != nil {
		return fail(std, err)
	}
	if err := checkSecretFiles(cfg); err != nil {
		return fail(std, err)
	}
	fmt.Fprintln(std.Out, "config ok")
	return ExitOK
}

func checkKinds(cfg *config.Config) error {
	for _, name := range slices.Sorted(maps.Keys(cfg.Resources)) {
		kind := cfg.Resources[name].Kind
		if _, ok := core.Lookup(kind); !ok {
			return fmt.Errorf("config: resources.%s.kind: %q is not a registered adapter kind", name, kind)
		}
	}
	return nil
}

func checkSecretFiles(cfg *config.Config) error {
	if _, err := os.Stat(paths.Secrets); errors.Is(err, fs.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	for _, name := range cfg.SecretNames() {
		if err := secret.CheckFile(filepath.Join(paths.Secrets, name), paths.SecretUID); err != nil {
			return fmt.Errorf("secret %s: %w", name, err)
		}
	}
	return nil
}
