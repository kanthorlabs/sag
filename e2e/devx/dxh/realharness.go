package dxh

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

const ClaudeCodeVersion = "2.1.332"
const PiVersion = "1.6.1"

func VersionGate(ctx context.Context, r Runner, name, required string) error {
	out, err := r.Output(ctx, name, "--version")
	if err != nil {
		return fmt.Errorf("prerequisite missing: %s %s required, cannot run: %w", name, required, err)
	}
	text := strings.TrimSpace(string(out))
	if !strings.Contains(text, required) {
		return fmt.Errorf("prerequisite missing: %s %s required, got %s", name, required, text)
	}
	return nil
}

var jwtPattern = regexp.MustCompile(`eyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]*`)

func FilterNoJWT(s string) string {
	return jwtPattern.ReplaceAllString(s, "<JWT>")
}
