//go:build !devxtest

package image

func Reference() (string, bool) {
	return productReference()
}
