package dxh_test

import (
	"context"
	"strconv"
	"strings"
	"syscall"
	"testing"

	"github.com/kanthorlabs/sag/e2e/devx/dxh"
)

func TestSetsidRunnerCreatesNewSession(t *testing.T) {
	r := dxh.NewSetsidRunner(t.TempDir(), "")
	out, err := r.Output(context.Background(), "sh", "-c", "ps -o sess= -p $$ | tr -d ' '")
	if err != nil {
		t.Fatalf("get child session id: %v", err)
	}
	childSid, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil {
		t.Fatalf("parse child session id %q: %v", string(out), err)
	}
	parentSid, _ := syscall.Getsid(0)
	if childSid == parentSid {
		t.Fatalf("child session id %d == parent session id %d; setsid was not effective", childSid, parentSid)
	}
}
