package cli

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kanthorlabs/sag/internal/approval"
	"github.com/kanthorlabs/sag/internal/core"
	"github.com/kanthorlabs/sag/internal/secret"
)

var approvalTestKey = bytes.Repeat([]byte{0x44}, 32)

type fakeInspector struct {
	target   approval.Target
	objects  approval.Objects
	err      error
	inspects int
}

func (f *fakeInspector) Kind() string                    { return "fake" }
func (f *fakeInspector) Capabilities() []core.Capability { return nil }
func (f *fakeInspector) Open(context.Context, core.ResourceConfig, secret.Credential) (core.Resource, error) {
	return nil, errors.ErrUnsupported
}
func (f *fakeInspector) Inspect(context.Context, core.ResourceConfig, secret.Credential) (approval.Target, approval.Objects, error) {
	f.inspects++
	return f.target, f.objects, f.err
}

type approveFixture struct {
	inspector *fakeInspector
	store     *approval.Store
	dir       string
	loads     int
}

func newApproveFixture(t *testing.T) *approveFixture {
	t.Helper()
	dir := filepath.Join(t.TempDir(), approval.DirName)
	f := &approveFixture{
		inspector: &fakeInspector{
			target: approval.Target{Resource: "ext", Host: "pg", Port: 5432, Database: "sag_ext", User: "postgres"},
			objects: approval.Objects{
				Extensions: []approval.Extension{{Name: "dblink", Version: "1.2", Schema: "public"}},
				Languages:  []string{"plperlu"},
			},
		},
		store: approval.NewStore(dir, approvalTestKey),
		dir:   dir,
	}
	saved := loadResource
	t.Cleanup(func() { loadResource = saved })
	loadResource = func(name string) (resourceTarget, error) {
		f.loads++
		if name != "ext" {
			return resourceTarget{}, errors.New("resource is not in the config")
		}
		return resourceTarget{Adapter: f.inspector, Config: core.ResourceConfig{Name: name}, Approvals: f.store}, nil
	}
	return f
}

func (f *approveFixture) fingerprint(t *testing.T) string {
	t.Helper()
	fp, err := approval.Fingerprint(f.inspector.target, f.inspector.objects)
	if err != nil {
		t.Fatalf("Fingerprint: %v", err)
	}
	return fp
}

func (f *approveFixture) recordExists() bool {
	_, err := os.Lstat(filepath.Join(f.dir, "ext.json"))
	return !errors.Is(err, fs.ErrNotExist)
}

func approveIO(input string, terminalIn, terminalOut bool) (IO, *bytes.Buffer, *bytes.Buffer) {
	out, errOut := &bytes.Buffer{}, &bytes.Buffer{}
	std := IO{In: strings.NewReader(input), Out: out, Err: errOut}
	std.IsTerminal = func(stream any) bool {
		return (terminalIn && stream == std.In) || (terminalOut && stream == std.Out)
	}
	return std, out, errOut
}

func TestApproveNeedsTerminal(t *testing.T) {
	for _, tc := range []struct{ in, out bool }{{false, true}, {true, false}, {false, false}} {
		f := newApproveFixture(t)
		std, out, errOut := approveIO("ext\n", tc.in, tc.out)
		if code := Run([]string{"extension", "approve", "ext"}, std); code != ExitFailure {
			t.Fatalf("in=%v out=%v: exit = %d; want %d", tc.in, tc.out, code, ExitFailure)
		}
		if !strings.Contains(errOut.String(), "terminal") {
			t.Fatalf("in=%v out=%v: stderr = %q; want a terminal message", tc.in, tc.out, errOut.String())
		}
		if out.Len() != 0 {
			t.Fatalf("in=%v out=%v: stdout = %q; want empty", tc.in, tc.out, out.String())
		}
		if f.loads != 0 || f.inspector.inspects != 0 || f.recordExists() {
			t.Fatalf("in=%v out=%v: command loaded, inspected, or wrote without a terminal", tc.in, tc.out)
		}
	}
}

func TestApproveWrongConfirmationStops(t *testing.T) {
	for _, input := range []string{"", "\n", "other\n", "EXT\n", "ext \n", " ext\n", "ex\n", "ext2\n", "yes\n", "ext\x00\n", "\rext\n"} {
		f := newApproveFixture(t)
		std, out, errOut := approveIO(input, true, true)
		if code := Run([]string{"extension", "approve", "ext"}, std); code != ExitFailure {
			t.Fatalf("input %q: exit = %d; want %d", input, code, ExitFailure)
		}
		if f.recordExists() {
			t.Fatalf("input %q: command wrote an approval record", input)
		}
		if !strings.Contains(out.String(), f.fingerprint(t)) {
			t.Fatalf("input %q: stdout = %q; want the fingerprint before the prompt", input, out.String())
		}
		if !strings.Contains(errOut.String(), "no approval") {
			t.Fatalf("input %q: stderr = %q; want a stop message", input, errOut.String())
		}
	}
}

func TestApproveWritesRecord(t *testing.T) {
	for _, input := range []string{"ext\n", "ext\r\n", "ext"} {
		f := newApproveFixture(t)
		std, out, errOut := approveIO(input, true, true)
		if code := Run([]string{"extension", "approve", "ext"}, std); code != ExitOK {
			t.Fatalf("input %q: exit = %d; want %d; stderr %q", input, code, ExitOK, errOut.String())
		}
		fp := f.fingerprint(t)
		if err := f.store.Verify("ext", fp); err != nil {
			t.Fatalf("input %q: Verify after approve: %v", input, err)
		}
		for _, want := range []string{"dblink", "1.2", "public", "plperlu", fp, "type the resource name"} {
			if !strings.Contains(out.String(), want) {
				t.Fatalf("input %q: stdout = %q; want %q", input, out.String(), want)
			}
		}
	}
}

func TestApproveNoRiskyObjects(t *testing.T) {
	f := newApproveFixture(t)
	f.inspector.objects = approval.Objects{}
	std, out, _ := approveIO("ext\n", true, true)
	if code := Run([]string{"extension", "approve", "ext"}, std); code != ExitOK {
		t.Fatalf("exit = %d; want %d", code, ExitOK)
	}
	if f.recordExists() {
		t.Fatal("command wrote a record for a resource without risky objects")
	}
	if !strings.Contains(out.String(), "no risky objects") {
		t.Fatalf("stdout = %q; want a no risky objects message", out.String())
	}
}

func TestApproveInspectErrorStops(t *testing.T) {
	f := newApproveFixture(t)
	f.inspector.err = errors.New("resource connect failed")
	std, _, errOut := approveIO("ext\n", true, true)
	if code := Run([]string{"extension", "approve", "ext"}, std); code != ExitFailure {
		t.Fatalf("exit = %d; want %d", code, ExitFailure)
	}
	if f.recordExists() || !strings.Contains(errOut.String(), "connect failed") {
		t.Fatalf("record written or stderr %q has no cause", errOut.String())
	}
}

func TestApproveUnknownResource(t *testing.T) {
	f := newApproveFixture(t)
	std, _, _ := approveIO("absent\n", true, true)
	if code := Run([]string{"extension", "approve", "absent"}, std); code != ExitFailure {
		t.Fatalf("exit = %d; want %d", code, ExitFailure)
	}
	if f.inspector.inspects != 0 {
		t.Fatal("command inspected an unknown resource")
	}
}

func TestApproveAdapterWithoutInspector(t *testing.T) {
	f := newApproveFixture(t)
	loadResource = func(name string) (resourceTarget, error) {
		return resourceTarget{Adapter: plainAdapter{}, Approvals: f.store}, nil
	}
	std, _, errOut := approveIO("ext\n", true, true)
	if code := Run([]string{"extension", "approve", "ext"}, std); code != ExitFailure {
		t.Fatalf("exit = %d; want %d", code, ExitFailure)
	}
	if f.recordExists() || errOut.Len() == 0 {
		t.Fatalf("record written or no error text: %q", errOut.String())
	}
}

func TestApproveBadUsage(t *testing.T) {
	for _, args := range [][]string{{}, {"a", "b"}} {
		f := newApproveFixture(t)
		std, _, errOut := approveIO("ext\n", true, true)
		if code := Run(append([]string{"extension", "approve"}, args...), std); code != ExitUsage {
			t.Fatalf("args %q: exit = %d; want %d", args, code, ExitUsage)
		}
		if !strings.Contains(errOut.String(), "usage") || f.loads != 0 {
			t.Fatalf("args %q: stderr = %q, loads = %d", args, errOut.String(), f.loads)
		}
	}
}

type plainAdapter struct{}

func (plainAdapter) Kind() string                    { return "plain" }
func (plainAdapter) Capabilities() []core.Capability { return nil }
func (plainAdapter) Open(context.Context, core.ResourceConfig, secret.Credential) (core.Resource, error) {
	return nil, errors.ErrUnsupported
}

func revokePaths(t *testing.T) string {
	t.Helper()
	saved := paths
	t.Cleanup(func() { paths = saved })
	paths.State = t.TempDir()
	return filepath.Join(paths.State, approval.DirName)
}

func TestRevokeRemovesRecord(t *testing.T) {
	dir := revokePaths(t)
	store := approval.NewStore(dir, approvalTestKey)
	fp := strings.Repeat("a", 64)
	if _, err := store.Save("ext", fp, approval.Objects{Languages: []string{"plperlu"}}, time.Now()); err != nil {
		t.Fatalf("Save: %v", err)
	}
	std, out, errOut := approveIO("", false, false)
	if code := Run([]string{"extension", "revoke", "ext"}, std); code != ExitOK {
		t.Fatalf("exit = %d; want %d; stderr %q", code, ExitOK, errOut.String())
	}
	if err := store.Verify("ext", fp); !errors.Is(err, approval.ErrNotApproved) {
		t.Fatalf("Verify after revoke = %v; want ErrNotApproved", err)
	}
	if !strings.Contains(out.String(), "ext") {
		t.Fatalf("stdout = %q; want the resource name", out.String())
	}
}

func TestRevokeWithoutRecordFails(t *testing.T) {
	revokePaths(t)
	std, _, errOut := approveIO("", false, false)
	if code := Run([]string{"extension", "revoke", "ext"}, std); code != ExitFailure {
		t.Fatalf("exit = %d; want %d", code, ExitFailure)
	}
	if errOut.Len() == 0 {
		t.Fatal("stderr is empty; want the cause")
	}
}

func TestRevokeBadUsage(t *testing.T) {
	revokePaths(t)
	for _, args := range [][]string{{}, {"a", "b"}} {
		std, _, errOut := approveIO("", false, false)
		if code := Run(append([]string{"extension", "revoke"}, args...), std); code != ExitUsage {
			t.Fatalf("args %q: exit = %d; want %d", args, code, ExitUsage)
		}
		if !strings.Contains(errOut.String(), "usage") {
			t.Fatalf("args %q: stderr = %q", args, errOut.String())
		}
	}
}
