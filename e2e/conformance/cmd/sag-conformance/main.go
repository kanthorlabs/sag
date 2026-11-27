package main

import (
	"os"

	"github.com/kanthorlabs/sag/e2e/conformance/passthrough"
	_ "github.com/kanthorlabs/sag/internal/adapter/postgres"
	"github.com/kanthorlabs/sag/internal/cli"
)

func main() {
	os.Exit(cli.RunWith(os.Args[1:], cli.StdIO(), passthrough.Wrap))
}
