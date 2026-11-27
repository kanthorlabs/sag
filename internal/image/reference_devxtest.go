//go:build devxtest

package image

import "github.com/kanthorlabs/sag/internal/testhook"

func Reference() (string, bool) {
	if img := testhook.Image(); img != "" {
		return img, false
	}
	return productReference()
}
