package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
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
	data, err := os.ReadFile(os.Args[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "fakeinstall: read %s: %v\n", os.Args[1], err)
		os.Exit(1)
	}
	h := sha256.Sum256(data)
	line := os.Args[1] + "\t" + hex.EncodeToString(h[:]) + "\n"
	f, err := os.OpenFile(record, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fakeinstall: open record: %v\n", err)
		os.Exit(1)
	}
	defer f.Close()
	if _, err := f.WriteString(line); err != nil {
		fmt.Fprintf(os.Stderr, "fakeinstall: write record: %v\n", err)
		os.Exit(1)
	}
}
