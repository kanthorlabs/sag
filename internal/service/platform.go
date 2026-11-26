package service

import "fmt"

var acceptedPlatforms = []string{"darwin/arm64"}

func unsupportedPlatformErr(goos, goarch string) error {
	return fmt.Errorf("unsupported platform %s/%s", goos, goarch)
}

func CheckPlatform(goos, goarch string) error {
	platform := goos + "/" + goarch
	for _, p := range acceptedPlatforms {
		if p == platform {
			return nil
		}
	}
	return unsupportedPlatformErr(goos, goarch)
}
