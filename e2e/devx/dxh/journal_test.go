package dxh_test

import (
	"context"
	"strings"
	"testing"

	"github.com/kanthorlabs/sag/e2e/devx/dxh"
)

func TestJournalUserScope(t *testing.T) {
	r := newFake()
	r.set("journalctl --user -u sag -n 50", []byte("log line\n"), nil)

	out, err := dxh.UserJournal(context.Background(), r, 50)
	if err != nil {
		t.Fatalf("UserJournal: %v", err)
	}
	if !strings.Contains(string(out), "log line") {
		t.Fatalf("unexpected output: %q", out)
	}

	var found bool
	for _, call := range r.calls {
		joined := strings.Join(call, " ")
		if strings.Contains(joined, "--user") && strings.Contains(joined, "journalctl") {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("UserJournal did not call journalctl with --user scope")
	}
}
