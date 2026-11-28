package deps

import (
	"fmt"
	"strings"
)

func aptVersionParse(raw string) (string, error) {
	if raw == "" {
		return "", fmt.Errorf("deps: apt: empty version string")
	}
	if i := strings.Index(raw, ":"); i >= 0 {
		raw = raw[i+1:]
	}
	if i := strings.IndexAny(raw, "-+~"); i >= 0 {
		raw = raw[:i]
	}
	if raw == "" {
		return "", fmt.Errorf("deps: apt: empty version after strip")
	}
	return raw, nil
}

func parseAptCachePolicy(output string) (string, error) {
	for _, line := range strings.Split(output, "\n") {
		trimmed := strings.TrimSpace(line)
		after, found := strings.CutPrefix(trimmed, "Candidate:")
		if !found {
			continue
		}
		candidate := strings.TrimSpace(after)
		if candidate == "(none)" || candidate == "" {
			return "", nil
		}
		return aptVersionParse(candidate)
	}
	return "", nil
}
