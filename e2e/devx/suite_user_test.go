//go:build devxe2e

package devx_test

import (
	"context"
	"testing"
	"time"
)

var userHarnessTests = map[string]func(*testing.T, context.Context){}

func TestDevxUser(t *testing.T) {
	t.Fatal("User suite: D9.1 guard not installed; run after D9.1 merges")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	userHarnessOrder := []string{
		"DXAC39_DefaultPath",
		"DXAC47_RealClaudeCode",
		"DXAC48_RealPi",
	}
	for _, name := range userHarnessOrder {
		name := name
		fn := userHarnessTests[name]
		t.Run(name, func(t *testing.T) {
			if fn == nil {
				t.Fatal("not implemented")
			}
			fn(t, ctx)
		})
	}
}
