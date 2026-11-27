package dxh

import (
	"context"
	"fmt"
	"strings"
	"time"
)

const idleTimeout = 90 * time.Second

func WaitLaunchAgentIdle(ctx context.Context, r Runner, uid string) error {
	deadline := time.Now().Add(idleTimeout)
	for {
		out, err := r.Output(ctx, "launchctl", "print", "gui/"+uid+"/net.kanthorlabs.sag")
		if err != nil || strings.Contains(string(out), "state = not running") {
			return nil
		}
		if time.Now().After(deadline) {
			var state []string
			for _, l := range strings.Split(string(out), "\n") {
				l = strings.TrimSpace(l)
				if strings.HasPrefix(l, "state = ") || strings.HasPrefix(l, "last exit code = ") || strings.HasPrefix(l, "runs = ") || strings.HasPrefix(l, "pid = ") {
					state = append(state, l)
				}
			}
			return fmt.Errorf("launch agent not idle after %s: %s", idleTimeout, strings.Join(state, "; "))
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
}
