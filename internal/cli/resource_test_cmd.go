package cli

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/kanthorlabs/sag/internal/core"
)

const (
	resourceTestUsage    = "usage: sag resource test <name>"
	preflightFailedCause = "preflight failed"
	resultPassedFormat   = "result: resource %s passed preflight\n"
	resultRefusedFormat  = "result: resource %s refused by preflight\n"
	resultFailedFormat   = "result: resource %s preflight failed: %s\n"
	checkPassed          = "pass"
	checkFailed          = "FAIL"
)

func resourceTest(args []string, std IO) int {
	if std.Mode == ModeHost {
		return hostResourceTest(args, std)
	}
	if len(args) != 1 {
		fmt.Fprintln(std.Err, resourceTestUsage)
		return ExitUsage
	}
	name := args[0]
	target, err := loadResource(name)
	if err != nil {
		return fail(std, err)
	}
	ctx := context.Background()
	res, err := target.Adapter.Open(ctx, target.Config, target.Cred)
	if err != nil {
		fmt.Fprintf(std.Out, resultFailedFormat, name, err)
		return ExitFailure
	}
	defer res.Close()
	report, err := res.Preflight(ctx)
	if err != nil {
		fmt.Fprintf(std.Out, resultFailedFormat, name, preflightFailure(err))
		return ExitFailure
	}
	printReport(std.Out, report)
	if !report.Passed {
		fmt.Fprintf(std.Out, resultRefusedFormat, name)
		return ExitFailure
	}
	fmt.Fprintf(std.Out, resultPassedFormat, name)
	return ExitOK
}

func printReport(w io.Writer, report core.PreflightReport) {
	for _, c := range report.Checks {
		status := checkFailed
		if c.Passed {
			status = checkPassed
		}
		fmt.Fprintf(w, "check %s: %s: %q\n", c.Name, status, c.Detail)
	}
	for _, warning := range report.Warnings {
		fmt.Fprintf(w, "warning: %s\n", warning)
	}
	if report.Hint != "" {
		fmt.Fprintf(w, "hint: %q\n", report.Hint)
	}
}

func preflightFailure(err error) string {
	var coreErr *core.Error
	if errors.As(err, &coreErr) {
		return coreErr.Error()
	}
	return preflightFailedCause
}
