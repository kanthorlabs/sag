package cli

import (
	"fmt"

	"github.com/kanthorlabs/sag/internal/image"
)

const versionUsage = "usage: sag version"

func version(args []string, std IO) int {
	if len(args) != 0 {
		fmt.Fprintln(std.Err, versionUsage)
		return ExitUsage
	}
	ref, _ := image.Reference()
	fmt.Fprintf(std.Out, "sag %s %s\n", Version, ref)
	return ExitOK
}
