package cli

import "errors"

func version(args []string, std IO) int {
	return fail(std, errors.ErrUnsupported)
}
