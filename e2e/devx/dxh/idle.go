package dxh

import (
	"context"
	"fmt"
	"strings"
	"time"
)

const idleTimeout = 30 * time.Second

func WaitLaunchAgentIdle(ctx context.Context, r Runner, uid string) error {
	deadline := time.Now().Add(idleTimeout)
	for {
		out, err := r.Output(ctx, "launchctl", "print", "gui/"+uid+"/net.kanthorlabs.sag")
		if err != nil || strings.Contains(string(out), "state = not running") {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("launch agent not idle after %s", idleTimeout)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
}
