package cli

import "fmt"

const versionUsage = "usage: sag version"

func version(args []string, std IO) int {
	if len(args) != 0 {
		fmt.Fprintln(std.Err, versionUsage)
		return ExitUsage
	}
	fmt.Fprintf(std.Out, "sag %s\n", Version)
	return ExitOK
}
