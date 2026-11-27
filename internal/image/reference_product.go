//go:build !devxtest

package image

func Reference() string {
	return productReference()
}
