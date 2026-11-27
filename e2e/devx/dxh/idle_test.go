package dxh

import (
	"context"
	"fmt"
	"testing"
)

type idleRunner struct {
	calls  []string
	output []byte
	err    error
}

func (r *idleRunner) Output(_ context.Context, name string, args ...string) ([]byte, error) {
	cmd := name
	for _, a := range args {
		cmd += " " + a
	}
	r.calls = append(r.calls, cmd)
	return r.output, r.err
}

func TestWaitLaunchAgentIdleWhenNotRunning(t *testing.T) {
	r := &idleRunner{output: []byte("state = not running\n")}
	if err := WaitLaunchAgentIdle(context.Background(), r, "1000"); err != nil {
		t.Fatalf("want nil, got %v", err)
	}
}

func TestWaitLaunchAgentIdleWhenPrintFails(t *testing.T) {
	r := &idleRunner{err: fmt.Errorf("exit status 113")}
	if err := WaitLaunchAgentIdle(context.Background(), r, "1000"); err != nil {
		t.Fatalf("want nil when launchctl fails, got %v", err)
	}
}
