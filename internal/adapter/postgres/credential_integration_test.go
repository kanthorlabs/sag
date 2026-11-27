//go:build integration

package postgres

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/kanthorlabs/sag/internal/core"
	"github.com/kanthorlabs/sag/internal/secret"
)

func TestCheckCredentialAgainstPG(t *testing.T) {
	pg := startPostgres(t)

	cfg := core.ResourceConfig{
		Kind:           Kind,
		TLS:            core.TLSVerifyFull,
		TLSCA:          pg.CAPEM,
		PoolSize:       1,
		ConnectTimeout: 10 * time.Second,
	}
	cred := secret.NewCredential("check", []byte(pg.HostDSN()))

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var a Adapter
	if err := a.CheckCredential(ctx, cfg, cred); err != nil {
		t.Fatalf("CheckCredential: %v", err)
	}
}

func TestCheckCredentialClosedPortErrors(t *testing.T) {
	markers := []string{"MARKER1", "MARKER2", "MARKER3"}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()

	dsn := dsnWithMarkers("MARKER1", "MARKER2", "127.0.0.1", port, "MARKER3")
	cred := secret.NewCredential("check", []byte(dsn))
	cfg := core.ResourceConfig{
		Kind:           Kind,
		TLS:            core.TLSDisable,
		PoolSize:       1,
		ConnectTimeout: time.Second,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	credErr := Adapter{}.CheckCredential(ctx, cfg, cred)
	if credErr == nil {
		t.Fatal("CheckCredential with closed port: want error, got nil")
	}
	for _, m := range markers {
		if strings.Contains(credErr.Error(), m) {
			t.Errorf("CheckCredential error %q contains marker %q", credErr.Error(), m)
		}
	}
}

func dsnWithMarkers(user, pass, host string, port int, db string) string {
	return "postgres://" + user + ":" + pass + "@" + host + ":" + portStr(port) + "/" + db
}

func portStr(port int) string {
	return net.JoinHostPort("", itoa(port))[1:]
}

func itoa(n int) string {
	const digits = "0123456789"
	if n == 0 {
		return "0"
	}
	buf := [20]byte{}
	pos := len(buf)
	for n > 0 {
		pos--
		buf[pos] = digits[n%10]
		n /= 10
	}
	return string(buf[pos:])
}
