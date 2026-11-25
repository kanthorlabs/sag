package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/kanthorlabs/sag/internal/approval"
)

const (
	extensionApproveUsage = "usage: sag extension approve <resource>"
	extensionRevokeUsage  = "usage: sag extension revoke <resource>"
	revokedFormat         = "approval revoked for resource %s; the resource stays served until the next preflight re-check or a daemon restart\n"
)

var (
	errApproveNoTerminal = errors.New("standard input and standard output must be a terminal; run the command with podman exec -it")
	errNoInspector       = errors.New("the resource kind does not support extension approval")
	errTargetMismatch    = errors.New("the inspected target does not name the resource")
	errNotConfirmed      = errors.New("confirmation does not match the resource name; no approval written")
)

func extensionApprove(args []string, std IO) int {
	if !std.IsTerminal(std.In) || !std.IsTerminal(std.Out) {
		return fail(std, errApproveNoTerminal)
	}
	if len(args) != 1 {
		fmt.Fprintln(std.Err, extensionApproveUsage)
		return ExitUsage
	}
	name := args[0]
	target, err := loadResource(name)
	if err != nil {
		return fail(std, err)
	}
	inspector, ok := target.Adapter.(approval.Inspector)
	if !ok {
		return fail(std, errNoInspector)
	}
	t, objects, err := inspector.Inspect(context.Background(), target.Config, target.Cred)
	if err != nil {
		return fail(std, err)
	}
	if t.Resource != name {
		return fail(std, errTargetMismatch)
	}
	if len(objects.Extensions) == 0 && len(objects.Languages) == 0 {
		fmt.Fprintf(std.Out, "resource %s has no risky objects; no approval needed\n", name)
		return ExitOK
	}
	fingerprint, err := approval.Fingerprint(t, objects)
	if err != nil {
		return fail(std, err)
	}
	printObjects(std.Out, name, objects, fingerprint)
	fmt.Fprintf(std.Out, "type the resource name %s to approve: ", name)
	if readConfirmation(std.In) != name {
		fmt.Fprintln(std.Out)
		return fail(std, errNotConfirmed)
	}
	if _, err := target.Approvals.Save(name, fingerprint, objects, time.Now()); err != nil {
		return fail(std, err)
	}
	fmt.Fprintf(std.Out, "approval written for resource %s, fingerprint %s\n", name, fingerprint)
	return ExitOK
}

func printObjects(w io.Writer, name string, o approval.Objects, fingerprint string) {
	fmt.Fprintf(w, "resource %s has risky objects:\n", name)
	for _, e := range o.Extensions {
		fmt.Fprintf(w, "  extension %q version %q schema %q\n", e.Name, e.Version, e.Schema)
	}
	for _, l := range o.Languages {
		fmt.Fprintf(w, "  language %q\n", l)
	}
	fmt.Fprintf(w, "fingerprint %s\n", fingerprint)
}

func readConfirmation(in io.Reader) string {
	line, err := bufio.NewReader(in).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return ""
	}
	line = strings.TrimSuffix(line, "\n")
	return strings.TrimSuffix(line, "\r")
}

func extensionRevoke(args []string, std IO) int {
	if len(args) != 1 {
		fmt.Fprintln(std.Err, extensionRevokeUsage)
		return ExitUsage
	}
	store := approval.NewStore(filepath.Join(paths.State, approval.DirName), nil)
	if err := store.Revoke(args[0]); err != nil {
		return fail(std, err)
	}
	fmt.Fprintf(std.Out, revokedFormat, args[0])
	return ExitOK
}
