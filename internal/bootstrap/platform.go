package bootstrap

import "fmt"

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
