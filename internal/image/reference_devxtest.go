//go:build devxtest

package image

import (
	"strings"

	"github.com/kanthorlabs/sag/internal/testhook"
)

func Reference() (string, bool) {
	if img := testhook.Image(); img != "" {
		return img, false
	}
	ref, devBuild := productReference()
	if reg := testhook.Registry(); reg != "" {
		ref = strings.Replace(ref, "docker.io", reg, 1)
	}
	return ref, devBuild
}
