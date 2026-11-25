package cli

import "errors"

func extensionApprove(args []string, std IO) int {
	return fail(std, errors.ErrUnsupported)
}

func extensionRevoke(args []string, std IO) int {
	return fail(std, errors.ErrUnsupported)
}
