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

type SkipError struct{}

func (e *SkipError) Error() string {
	return "pins: empty or comment line"
}

func ParseLine(line string) (Item, error) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return Item{}, &SkipError{}
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

func ValidateItem(item Item, requireHTTPS bool) error {
	switch item.Source {
	case "release", "apt":
	default:
		return fmt.Errorf("pins: unknown source %q for %s", item.Source, item.Name)
	}
	if item.SHA256 != "-" {
		if len(item.SHA256) != 64 {
			return fmt.Errorf("pins: sha256 for %s must be 64 hex chars or -", item.Name)
		}
		for _, c := range item.SHA256 {
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
				return fmt.Errorf("pins: sha256 for %s is not hex", item.Name)
			}
		}
	}
	if requireHTTPS && item.Source == "release" && !strings.HasPrefix(item.Locator, "https://") {
		return fmt.Errorf("pins: locator for %s must use https scheme", item.Name)
	}
	return nil
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
