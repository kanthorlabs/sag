package daemon

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

func TestContainerfileBaseImagesPinned(t *testing.T) {
	data, err := os.ReadFile("../../deploy/Containerfile")
	if err != nil {
		t.Fatal(err)
	}
	pinned := regexp.MustCompile(`^FROM [a-z0-9./-]+:[A-Za-z0-9._-]+@sha256:[0-9a-f]{64}( AS [a-z]+)?$`)
	count := 0
	for line := range strings.Lines(string(data)) {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "FROM ") {
			continue
		}
		count++
		if !pinned.MatchString(line) {
			t.Errorf("base image is not pinned by tag and index digest: %s", line)
		}
	}
	if count != 2 {
		t.Fatalf("FROM lines = %d; want 2", count)
	}
}
