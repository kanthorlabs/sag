package pins

import (
	"fmt"
	"strings"
)

type Item struct {
	Name     string
	Version  string
	Platform string
	Source   string
	Locator  string
	SHA256   string
}

func ParseLine(line string) (Item, error) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return Item{}, fmt.Errorf("pins: empty or comment line")
	}
	fields := strings.Fields(line)
	if len(fields) != 6 {
		return Item{}, fmt.Errorf("pins: line has %d fields, want 6: %q", len(fields), line)
	}
	return Item{
		Name:     fields[0],
		Version:  fields[1],
		Platform: fields[2],
		Source:   fields[3],
		Locator:  fields[4],
		SHA256:   fields[5],
	}, nil
}

func SelectPlatform(items []Item, goos, goarch string) []Item {
	target := goos + "/" + goarch
	var out []Item
	for _, it := range items {
		if it.Platform == target || it.Platform == goos+"/*" {
			out = append(out, it)
		}
	}
	return out
}
