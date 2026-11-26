package cli

import "errors"

var errNotImplemented = errors.New("not implemented")

func bootstrapCmd(args []string, std IO) int {
	return fail(std, errNotImplemented)
}
