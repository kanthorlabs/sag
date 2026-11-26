package main

import (
	"os"
)

func main() {
	if len(os.Args) < 2 {
		os.Exit(0)
	}
	record := os.Getenv("SAG_TEST_FAKEINSTALL_RECORD")
	if record == "" {
		os.Exit(0)
	}
	if err := os.WriteFile(record, []byte(os.Args[1]), 0o600); err != nil {
		os.Exit(1)
	}
}
