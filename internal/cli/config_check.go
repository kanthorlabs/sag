package cli

import "errors"

func configCheck(args []string, std IO) int {
	return fail(std, errors.ErrUnsupported)
}
