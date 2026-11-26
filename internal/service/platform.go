package service

import "fmt"

func unsupportedPlatformErr(goos, goarch string) error {
	return fmt.Errorf("unsupported platform %s/%s", goos, goarch)
}
