package main

import (
	"os"

	_ "github.com/kanthorlabs/sag/internal/adapter/postgres"
	"github.com/kanthorlabs/sag/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:], cli.StdIO()))
}
