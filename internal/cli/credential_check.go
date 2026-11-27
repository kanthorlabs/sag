package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"time"

	"github.com/kanthorlabs/sag/internal/core"
	"github.com/kanthorlabs/sag/internal/secret"
)

const checkUsage = "usage: sag credential check --kind <kind> --tls <mode>"

const checkConnectTimeout = 10 * time.Second

var (
	errCheckUnknownKind  = errors.New("credential check: unknown kind")
	errCheckNotCredKind  = errors.New("credential check: kind does not support credential check")
	errCheckInvalidInput = errors.New("credential check: invalid input")
)

var runCredentialCheck = func(ctx context.Context, cfg core.ResourceConfig, cred secret.Credential) error {
	a, ok := core.Lookup(cfg.Kind)
	if !ok {
		return errCheckUnknownKind
	}
	ck, ok := a.(core.CredentialKind)
	if !ok {
		return errCheckNotCredKind
	}
	return ck.CheckCredential(ctx, cfg, cred)
}

type checkStdinInput struct {
	Value string  `json:"value"`
	TLSCA *string `json:"tls_ca"`
}

func credentialCheck(args []string, std IO) int {
	fs := flag.NewFlagSet("credential check", flag.ContinueOnError)
	fs.SetOutput(std.Err)
	kind := fs.String("kind", "", "")
	tls := fs.String("tls", "", "")
	if err := fs.Parse(args); err != nil {
		return ExitUsage
	}
	if *kind == "" || *tls == "" {
		fmt.Fprintln(std.Err, checkUsage)
		return ExitUsage
	}

	var input checkStdinInput
	if err := json.NewDecoder(std.In).Decode(&input); err != nil {
		return fail(std, errCheckInvalidInput)
	}

	var caBytes []byte
	if input.TLSCA != nil && *input.TLSCA != "" {
		caBytes = []byte(*input.TLSCA)
	}
	cfg := core.ResourceConfig{
		Kind:           *kind,
		TLS:            core.TLSMode(*tls),
		TLSCA:          caBytes,
		PoolSize:       1,
		ConnectTimeout: checkConnectTimeout,
	}
	cred := secret.NewCredential("check", []byte(input.Value))

	if err := runCredentialCheck(context.Background(), cfg, cred); err != nil {
		return fail(std, err)
	}
	return ExitOK
}
