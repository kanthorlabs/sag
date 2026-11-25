package cli

import "errors"

func jwtGenerate(args []string, std IO) int {
	return fail(std, errors.ErrUnsupported)
}
