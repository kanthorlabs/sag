package bootstrap

import (
	"fmt"
	"os"
	"strings"
)

var acceptedPlatforms = []string{"darwin/arm64", "linux/amd64", "linux/arm64"}

func checkPlatform(goos, goarch string) error {
	platform := goos + "/" + goarch
	for _, p := range acceptedPlatforms {
		if p == platform {
			return nil
		}
	}
	return fmt.Errorf("unsupported platform %s", platform)
}

func checkLinuxDistro(goos, goarch, id, versionID string) error {
	if goos != "linux" {
		return nil
	}
	if id == "debian" && versionID == "13" {
		return nil
	}
	return fmt.Errorf("unsupported platform %s/%s", goos, goarch)
}

func readOSRelease() (id, versionID string, err error) {
	data, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return "", "", err
	}
	return parseOSRelease(string(data))
}

func parseOSRelease(content string) (id, versionID string, _ error) {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		val = strings.Trim(val, `"`)
		switch key {
		case "ID":
			id = val
		case "VERSION_ID":
			versionID = val
		}
	}
	return id, versionID, nil
}
