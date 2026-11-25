package daemon

import (
	"context"
	"io"
	"log/slog"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgproto3"

	_ "github.com/kanthorlabs/sag/internal/adapter/postgres"
	"github.com/kanthorlabs/sag/internal/config"
	"github.com/kanthorlabs/sag/internal/inventory"
	"github.com/kanthorlabs/sag/internal/secret"
)

func stalledServer(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go stall(c)
		}
	}()
	return ln.Addr().String()
}

func stall(c net.Conn) {
	defer c.Close()
	be := pgproto3.NewBackend(c, c)
	if _, err := be.ReceiveStartupMessage(); err != nil {
		return
	}
	be.Send(&pgproto3.AuthenticationOk{})
	be.Send(&pgproto3.BackendKeyData{ProcessID: 1, SecretKey: []byte{0, 0, 0, 1}})
	be.Send(&pgproto3.ReadyForQuery{TxStatus: 'I'})
	if err := be.Flush(); err != nil {
		return
	}
	io.Copy(io.Discard, c)
}

func openOne(t *testing.T, dsn string) (*inventory.Inventory, string) {
	t.Helper()
	cfg, err := config.Parse([]byte(testConfig + "    limits:\n      connect_timeout: 1s\n"))
	if err != nil {
		t.Fatal(err)
	}
	creds := map[string]secret.Credential{"sag-main": secret.NewCredential("sag-main", []byte(dsn))}
	var out strings.Builder
	inv := inventory.New()
	opened, err := openResources(context.Background(), slog.New(slog.NewJSONHandler(&out, nil)), cfg, creds, inv)
	closeResources(slog.New(slog.NewJSONHandler(io.Discard, nil)), opened)
	if err != nil {
		t.Fatalf("openResources: %v", err)
	}
	return inv, out.String()
}

func TestPreflightStalledServerTimesOut(t *testing.T) {
	saved := preflightSlack
	preflightSlack = 500 * time.Millisecond
	t.Cleanup(func() { preflightSlack = saved })

	dsn := "postgres://u:" + testDSNPassword + "@" + stalledServer(t) + "/app"
	start := time.Now()
	inv, log := openOne(t, dsn)
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Fatalf("openResources took %s; want the preflight deadline", elapsed)
	}
	if _, err := inv.Lookup("skeleton", "main"); err == nil {
		t.Fatal("resource main is served; want unavailable")
	}
	if !strings.Contains(log, `"cause":"preflight timed out"`) {
		t.Fatalf("log lacks the timeout cause:\n%s", log)
	}
	if strings.Contains(log, testDSNPassword) {
		t.Fatal("log holds the DSN password")
	}
}

func TestOpenFailureLogsCause(t *testing.T) {
	_, log := openOne(t, testDSN+"\n")
	if !strings.Contains(log, `"msg":"resource open failed"`) || !strings.Contains(log, "connection secret is not a postgres URL") {
		t.Fatalf("log lacks the open cause:\n%s", log)
	}
	if strings.Contains(log, testDSNPassword) || strings.Contains(log, "db.example.net") {
		t.Fatal("log holds the DSN")
	}
}
