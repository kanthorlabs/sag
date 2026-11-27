package cli

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kanthorlabs/sag/internal/core"
	"github.com/kanthorlabs/sag/internal/secret"
)

func checkIO() (IO, *strings.Builder, *strings.Builder) {
	out := &strings.Builder{}
	errOut := &strings.Builder{}
	return IO{
		In:         strings.NewReader(""),
		Out:        out,
		Err:        errOut,
		IsTerminal: func(any) bool { return false },
		Mode:       ModeContainer,
	}, out, errOut
}

func TestReadsStdinJSON(t *testing.T) {
	var gotCfg core.ResourceConfig
	var gotCred secret.Credential

	saved := runCredentialCheck
	defer func() { runCredentialCheck = saved }()
	runCredentialCheck = func(_ context.Context, cfg core.ResourceConfig, cred secret.Credential) error {
		gotCfg = cfg
		gotCred = cred
		return nil
	}

	input := `{"value":"postgres://user:pass@host:5432/db","tls_ca":null}`
	std, _, _ := checkIO()
	std.In = strings.NewReader(input)

	code := credentialCheck([]string{"--kind", "postgres", "--tls", "verify-full"}, std)
	if code != ExitOK {
		t.Fatalf("credentialCheck exit %d; want %d", code, ExitOK)
	}
	if string(gotCred.Reveal()) != "postgres://user:pass@host:5432/db" {
		t.Errorf("cred = %q; want DSN", string(gotCred.Reveal()))
	}
	if gotCfg.Kind != "postgres" {
		t.Errorf("cfg.Kind = %q; want postgres", gotCfg.Kind)
	}
	if gotCfg.TLS != core.TLSVerifyFull {
		t.Errorf("cfg.TLS = %q; want verify-full", gotCfg.TLS)
	}
	if len(gotCfg.TLSCA) != 0 {
		t.Errorf("cfg.TLSCA = %v; want empty (tls_ca was null)", gotCfg.TLSCA)
	}
}

func TestReadsStdinJSONWithTLSCA(t *testing.T) {
	var gotCfg core.ResourceConfig

	saved := runCredentialCheck
	defer func() { runCredentialCheck = saved }()
	runCredentialCheck = func(_ context.Context, cfg core.ResourceConfig, _ secret.Credential) error {
		gotCfg = cfg
		return nil
	}

	input := `{"value":"postgres://u:p@h:5432/db","tls_ca":"-----BEGIN CERTIFICATE-----\nABC\n-----END CERTIFICATE-----\n"}`
	std, _, _ := checkIO()
	std.In = strings.NewReader(input)

	code := credentialCheck([]string{"--kind", "postgres", "--tls", "verify-full"}, std)
	if code != ExitOK {
		t.Fatalf("credentialCheck exit %d; want %d", code, ExitOK)
	}
	if len(gotCfg.TLSCA) == 0 {
		t.Error("cfg.TLSCA is empty; want CA bytes")
	}
}

func TestFixedErrorExit1(t *testing.T) {
	marker := "MARKER_SECRET_VALUE"
	malformed := `{"value":"` + marker + `", broken json here`

	std, _, errOut := checkIO()
	std.In = strings.NewReader(malformed)

	code := credentialCheck([]string{"--kind", "postgres", "--tls", "disable"}, std)
	if code != ExitFailure {
		t.Fatalf("credentialCheck exit %d; want %d", code, ExitFailure)
	}
	if strings.Contains(errOut.String(), marker) {
		t.Errorf("stderr = %q; contains marker %q", errOut.String(), marker)
	}
}

func TestCheckCredentialFailureExit1(t *testing.T) {
	fixedErr := errors.New("credential check: cannot connect")

	saved := runCredentialCheck
	defer func() { runCredentialCheck = saved }()
	runCredentialCheck = func(_ context.Context, _ core.ResourceConfig, _ secret.Credential) error {
		return fixedErr
	}

	input := `{"value":"postgres://u:p@host:5432/db","tls_ca":null}`
	std, _, errOut := checkIO()
	std.In = strings.NewReader(input)

	code := credentialCheck([]string{"--kind", "postgres", "--tls", "disable"}, std)
	if code != ExitFailure {
		t.Fatalf("credentialCheck exit %d; want %d", code, ExitFailure)
	}
	if !strings.Contains(errOut.String(), "credential check: cannot connect") {
		t.Errorf("stderr = %q; want error text", errOut.String())
	}
}

func TestMissingFlagsExit2(t *testing.T) {
	cases := [][]string{
		{"--tls", "disable"},
		{"--kind", "postgres"},
		{},
	}
	for _, args := range cases {
		std, _, _ := checkIO()
		std.In = strings.NewReader(`{"value":"v","tls_ca":null}`)
		code := credentialCheck(args, std)
		if code != ExitUsage {
			t.Errorf("credentialCheck(%v) exit %d; want %d", args, code, ExitUsage)
		}
	}
}
