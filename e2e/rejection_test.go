//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/kanthorlabs/sag/e2e/harness"
)

const rejectionPrefix = "statement rejected: "

var closedDetailRules = map[string]bool{"statement_count": true, "statement": true, "node": true, "depth": true}

func rejectionMessage(reason string) string {
	rule, _, _ := strings.Cut(reason, ":")
	if closedDetailRules[rule] {
		return rejectionPrefix + reason
	}
	return rejectionPrefix + rule
}

func assertAuditReasons(ctx context.Context, t *testing.T, env harness.Env, container string, want map[string]string) {
	t.Helper()
	lines, err := env.Audit(ctx, container)
	if err != nil {
		t.Fatalf("audit copy: %v", err)
	}
	got := map[string]string{}
	for _, line := range lines {
		var rec struct {
			RequestID string `json:"request_id"`
			Reason    string `json:"reason"`
		}
		if err := json.Unmarshal(line, &rec); err != nil {
			t.Fatalf("audit line %s: %v", line, err)
		}
		got[rec.RequestID] = rec.Reason
	}
	for id, reason := range want {
		if got[id] != "query_rejected:"+reason {
			t.Errorf("audit reason of request %s = %q; want %q", id, got[id], "query_rejected:"+reason)
		}
	}
}
