package cli

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

const auditDateFormat = "2006-01-02"

func auditRead(args []string, std IO) int {
	defaultDate := time.Now().UTC().Format(auditDateFormat)
	fs := flag.NewFlagSet("audit read", flag.ContinueOnError)
	fs.SetOutput(std.Err)
	date := fs.String("date", defaultDate, "")
	if err := fs.Parse(args); err != nil {
		return ExitUsage
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(std.Err, "sag: audit read: unexpected arguments")
		return ExitUsage
	}

	if _, err := time.Parse(auditDateFormat, *date); err != nil {
		fmt.Fprintf(std.Err, "sag: audit read: invalid date %q: use YYYY-MM-DD\n", *date)
		return ExitUsage
	}

	auditPath := filepath.Join(paths.State, "audit", "audit-"+*date+".jsonl")
	f, err := os.Open(auditPath)
	if os.IsNotExist(err) {
		return ExitOK
	}
	if err != nil {
		return fail(std, err)
	}
	defer f.Close()

	if _, err := io.Copy(std.Out, f); err != nil {
		return fail(std, err)
	}
	return ExitOK
}
