package cli

import "errors"

func serve(args []string, std IO) int {
	return fail(std, errors.ErrUnsupported)
}
