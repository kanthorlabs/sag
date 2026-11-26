package postgres

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/kanthorlabs/sag/internal/core"
	"github.com/kanthorlabs/sag/internal/secret"
)

func TestValidateMatchesParseDSN(t *testing.T) {
	cases := []struct {
		dsn     string
		wantErr bool
	}{
		{"postgres://user:pass@host:5432/db", false},
		{"postgresql://user:pass@host:5432/db", false},
		{"postgres://user:pass@host/db", false},
		{"not-a-url", true},
		{"postgres://user:pass@host:5432/db?foo=bar", true},
		{"postgres://host1,host2:5432/db", true},
		{"postgres://user:pass@/db", true},
		{"postgres://user:pass@host/", true},
		{"postgres://user:pass@host:5432/db/extra", true},
	}
	for _, tc := range cases {
		cred := secret.NewCredential("test", []byte(tc.dsn))
		_, credErr := Adapter{}.ValidateCredential(cred)
		_, parseErr := parseDSN(tc.dsn)
		if (credErr != nil) != (parseErr != nil) {
			t.Errorf("DSN %q: ValidateCredential err=%v parseDSN err=%v; want same outcome", tc.dsn, credErr, parseErr)
		}
	}
}

func TestLoopbackHosts(t *testing.T) {
	loopback := []string{
		"postgres://u:p@localhost:5432/db",
		"postgres://u:p@127.0.0.1:5432/db",
		"postgres://u:p@127.1.2.3:5432/db",
		"postgres://u:p@[::1]:5432/db",
	}
	for _, dsn := range loopback {
		cred := secret.NewCredential("test", []byte(dsn))
		notes, err := Adapter{}.ValidateCredential(cred)
		if err != nil {
			t.Errorf("ValidateCredential(%q) error: %v", dsn, err)
			continue
		}
		if !notes.LoopbackHost {
			t.Errorf("ValidateCredential(%q).LoopbackHost = false; want true", dsn)
		}
	}

	nonLoopback := []string{
		"postgres://u:p@host.containers.internal:5432/db",
		"postgres://u:p@192.168.1.1:5432/db",
		"postgres://u:p@[2001:db8::1]:5432/db",
		"postgres://u:p@myhost:5432/db",
	}
	for _, dsn := range nonLoopback {
		cred := secret.NewCredential("test", []byte(dsn))
		notes, err := Adapter{}.ValidateCredential(cred)
		if err != nil {
			t.Errorf("ValidateCredential(%q) error: %v", dsn, err)
			continue
		}
		if notes.LoopbackHost {
			t.Errorf("ValidateCredential(%q).LoopbackHost = true; want false", dsn)
		}
	}
}

func TestErrorsFixedText(t *testing.T) {
	markers := []string{"MARKER1", "MARKER2", "MARKER3"}

	checkNoMarker := func(label, text string) {
		t.Helper()
		for _, m := range markers {
			if strings.Contains(text, m) {
				t.Errorf("%s = %q; contains marker %q", label, text, m)
			}
		}
	}

	invalidDSN := "postgres://MARKER1:MARKER2@host:5432/MARKER3?invalid=1"
	cred := secret.NewCredential("test", []byte(invalidDSN))
	_, err := Adapter{}.ValidateCredential(cred)
	if err != nil {
		checkNoMarker("ValidateCredential error", err.Error())
	} else {
		t.Error("ValidateCredential with invalid DSN: want error, got nil")
	}

	ln, lnErr := net.Listen("tcp", "127.0.0.1:0")
	if lnErr != nil {
		t.Fatal(lnErr)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()

	checkDSN := fmt.Sprintf("postgres://MARKER1:MARKER2@127.0.0.1:%d/MARKER3", port)
	checkCred := secret.NewCredential("test", []byte(checkDSN))
	cfg := core.ResourceConfig{
		Kind:           Kind,
		TLS:            core.TLSDisable,
		PoolSize:       1,
		ConnectTimeout: time.Second,
	}
	err = Adapter{}.CheckCredential(context.Background(), cfg, checkCred)
	if err != nil {
		checkNoMarker("CheckCredential error", err.Error())
	} else {
		t.Error("CheckCredential with closed port: want error, got nil")
	}
}

func TestCredentialCheckServerEngineError(t *testing.T) {
	fake := &catalogFake{answers: map[string][][]any{
		querySearchPath: {{"pg_catalog"}},
		queryServer:     {{"MySQL 8.0.23", "80023"}},
	}}
	err := credentialCheckServer(context.Background(), fake)
	if !errors.Is(err, errCheckEngine) {
		t.Fatalf("credentialCheckServer non-PG: got %v; want errCheckEngine", err)
	}
}

func TestCredentialCheckServerVersionError(t *testing.T) {
	fake := &catalogFake{answers: map[string][][]any{
		querySearchPath: {{"pg_catalog"}},
		queryServer:     {{"PostgreSQL 13.14 on x86_64-pc-linux-gnu", "130014"}},
	}}
	err := credentialCheckServer(context.Background(), fake)
	if !errors.Is(err, errCheckVersion) {
		t.Fatalf("credentialCheckServer old PG: got %v; want errCheckVersion", err)
	}
}

func TestCredentialCheckServerConnectError(t *testing.T) {
	fake := &catalogFake{failOn: querySearchPath, answers: map[string][][]any{}}
	err := credentialCheckServer(context.Background(), fake)
	if !errors.Is(err, errCheckConnect) {
		t.Fatalf("credentialCheckServer query fail: got %v; want errCheckConnect", err)
	}
}
