package cli

import "testing"

func TestServeRejectsArgs(t *testing.T) {
	std, _ := testIO()
	if code := Run([]string{"serve", "extra"}, std); code != ExitUsage {
		t.Fatalf("Run(serve extra) = %d; want %d", code, ExitUsage)
	}
}
