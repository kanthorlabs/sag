//go:build devxtest

package image

import "github.com/kanthorlabs/sag/internal/testhook"

func Reference() string {
	if img := testhook.Image(); img != "" {
		return img
	}
	return productReference()
}
