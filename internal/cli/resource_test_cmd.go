package cli

import "errors"

func resourceTest(args []string, std IO) int {
	return fail(std, errors.ErrUnsupported)
}
