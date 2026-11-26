package deps_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kanthorlabs/sag/internal/deps"
	"github.com/kanthorlabs/sag/internal/pins"
	"github.com/kanthorlabs/sag/internal/stepline"
)

func testItem(version string) pins.Item {
	return pins.Item{
		Name: "testpkg", Version: version, Platform: "darwin/arm64",
		Source: "release", Locator: "https://example.com/pkg.pkg", SHA256: "abc123",
	}
}

func newOut(w io.Writer) stepline.Writer {
	return stepline.New(w)
}

func fileHash(t *testing.T, path string) string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func TestNewerWarns(t *testing.T) {
	var buf strings.Builder
	d := deps.Deps{
		PodmanVersion: func(_ context.Context, _ string) (string, error) {
			return "6.0.0", nil
		},
		IsAbsent: func() bool { return false },
		Out:      newOut(&buf),
	}

	plan, err := d.Make(context.Background(), []pins.Item{testItem("5.0.0")}, "/usr/bin/podman", nil)
	if err != nil {
		t.Fatalf("Make: %v", err)
	}
	if len(plan.Items) != 1 || !plan.Items[0].Newer {
		t.Fatalf("plan.Items = %+v; want one Newer item", plan.Items)
	}
	if !strings.Contains(buf.String(), "warn testpkg") {
		t.Fatalf("output = %q; want 'warn testpkg' line", buf.String())
	}
}

func TestNewerVersionComparison_NumericNotLexical(t *testing.T) {
	var buf strings.Builder
	d := deps.Deps{
		PodmanVersion: func(_ context.Context, _ string) (string, error) {
			return "5.10.0", nil
		},
		IsAbsent: func() bool { return false },
		Out:      newOut(&buf),
	}

	plan, err := d.Make(context.Background(), []pins.Item{testItem("5.9.0")}, "/usr/bin/podman", nil)
	if err != nil {
		t.Fatalf("Make with 5.10 > 5.9: %v", err)
	}
	if len(plan.Items) != 1 || !plan.Items[0].Newer {
		t.Fatalf("5.10 vs 5.9: plan = %+v; want Newer=true (5.10 > 5.9 numerically)", plan.Items)
	}
}

func TestOlderFailsInPlan(t *testing.T) {
	var buf strings.Builder
	d := deps.Deps{
		PodmanVersion: func(_ context.Context, _ string) (string, error) {
			return "4.0.0", nil
		},
		IsAbsent: func() bool { return false },
		Out:      newOut(&buf),
	}

	_, err := d.Make(context.Background(), []pins.Item{testItem("5.0.0")}, "/usr/bin/podman", nil)
	var olderErr *deps.OlderError
	if !errors.As(err, &olderErr) {
		t.Fatalf("Make err = %v; want OlderError", err)
	}
	if !strings.Contains(buf.String(), "fail testpkg") {
		t.Fatalf("output = %q; want 'fail testpkg' line", buf.String())
	}
}

func TestPreApprovalOtherVersionIgnored(t *testing.T) {
	var buf strings.Builder
	d := deps.Deps{
		IsAbsent:    func() bool { return true },
		Out:         newOut(&buf),
		HasTerminal: func() bool { return false },
	}

	_, err := d.Make(context.Background(), []pins.Item{testItem("5.0.0")}, "", []string{"testpkg@4.0.0"})
	var refusal *deps.InstallRefusal
	if !errors.As(err, &refusal) {
		t.Fatalf("Make err = %v; want InstallRefusal", err)
	}
	if !strings.Contains(buf.String(), "warn testpkg") {
		t.Fatalf("output = %q; want 'warn testpkg' line for wrong version", buf.String())
	}
	if !strings.Contains(buf.String(), "4.0.0") {
		t.Fatalf("output = %q; want pre-approval version in warn", buf.String())
	}
}

func TestChecksumMismatchDeletes(t *testing.T) {
	var downloadedPath string

	d := deps.Deps{
		IsAbsent: func() bool { return true },
		Out:      newOut(io.Discard),
		Download: func(_ context.Context, _, tmpDir string) (string, error) {
			dst := filepath.Join(tmpDir, "pkg.pkg")
			if err := os.WriteFile(dst, []byte("fake content"), 0o600); err != nil {
				return "", err
			}
			downloadedPath = dst
			return dst, nil
		},
		SHA256: func(_ string) (string, error) {
			return "wronghash", nil
		},
		Install: func(_ context.Context, _ string, _ bool) error {
			t.Fatal("Install was called after checksum mismatch")
			return nil
		},
		StopBeforeInstall: func() bool { return false },
		HasTerminal:       func() bool { return false },
	}
	item := pins.Item{
		Name: "testpkg", Version: "5.0.0", Platform: "darwin/arm64",
		Source: "release", Locator: "https://example.com/pkg.pkg", SHA256: "correcthash",
	}
	plan := deps.Plan{Items: []deps.PlanItem{{Pin: item, Absent: true, Approve: true}}}

	_, err := d.Apply(context.Background(), plan)
	if err == nil {
		t.Fatal("Apply: want error for checksum mismatch, got nil")
	}
	if !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("Apply error = %q; want checksum mismatch", err.Error())
	}
	if downloadedPath != "" {
		if _, statErr := os.Stat(downloadedPath); statErr == nil {
			t.Fatal("Apply: downloaded file still exists after checksum mismatch; want deleted")
		}
	}
}

func TestInstallRunsOnVerifiedPath(t *testing.T) {
	content := []byte("fake pkg content")
	h := sha256.Sum256(content)
	correctHash := hex.EncodeToString(h[:])

	var downloadedPath string
	var installedPath string

	d := deps.Deps{
		IsAbsent: func() bool { return true },
		Out:      newOut(io.Discard),
		Download: func(_ context.Context, _, tmpDir string) (string, error) {
			dst := filepath.Join(tmpDir, "pkg.pkg")
			if err := os.WriteFile(dst, content, 0o600); err != nil {
				return "", err
			}
			downloadedPath = dst
			return dst, nil
		},
		SHA256: func(path string) (string, error) {
			return fileHash(t, path), nil
		},
		StopBeforeInstall: func() bool { return false },
		HasTerminal:       func() bool { return false },
		Install: func(_ context.Context, path string, _ bool) error {
			installedPath = path
			return nil
		},
		MachineList: func(_ context.Context, _ string) ([]deps.MachineInfo, error) {
			return []deps.MachineInfo{{Name: "default", Running: true}}, nil
		},
	}

	item := pins.Item{
		Name: "testpkg", Version: "5.0.0", Platform: "darwin/arm64",
		Source: "release", Locator: "https://example.com/pkg.pkg", SHA256: correctHash,
	}
	plan := deps.Plan{Items: []deps.PlanItem{{Pin: item, Absent: true, Approve: true}}}

	_, err := d.Apply(context.Background(), plan)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if installedPath == "" {
		t.Fatal("Apply: install was not called")
	}
	if installedPath != downloadedPath {
		t.Fatalf("Apply: install got %q; want exact downloaded path %q", installedPath, downloadedPath)
	}
}

func TestSudoNonInteractive(t *testing.T) {
	content := []byte("fake pkg content")
	h := sha256.Sum256(content)
	correctHash := hex.EncodeToString(h[:])

	var noInteractiveReceived bool
	d := deps.Deps{
		IsAbsent: func() bool { return true },
		Out:      newOut(io.Discard),
		Download: func(_ context.Context, _, tmpDir string) (string, error) {
			dst := filepath.Join(tmpDir, "pkg.pkg")
			if err := os.WriteFile(dst, content, 0o600); err != nil {
				return "", err
			}
			return dst, nil
		},
		SHA256: func(path string) (string, error) {
			return fileHash(t, path), nil
		},
		StopBeforeInstall: func() bool { return false },
		HasTerminal:       func() bool { return false },
		Install: func(_ context.Context, _ string, noInteractive bool) error {
			noInteractiveReceived = noInteractive
			return nil
		},
		MachineList: func(_ context.Context, _ string) ([]deps.MachineInfo, error) {
			return []deps.MachineInfo{{Name: "default", Running: true}}, nil
		},
	}

	item := pins.Item{
		Name: "testpkg", Version: "5.0.0", Platform: "darwin/arm64",
		Source: "release", Locator: "https://example.com/pkg.pkg", SHA256: correctHash,
	}
	plan := deps.Plan{Items: []deps.PlanItem{{Pin: item, Absent: true, Approve: true}}}

	if _, err := d.Apply(context.Background(), plan); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if !noInteractiveReceived {
		t.Fatal("Apply: noInteractive was not set to true when HasTerminal returns false")
	}

	var interactiveReceived bool
	d.HasTerminal = func() bool { return true }
	d.Install = func(_ context.Context, _ string, noInteractive bool) error {
		interactiveReceived = !noInteractive
		return nil
	}
	if _, err := d.Apply(context.Background(), plan); err != nil {
		t.Fatalf("Apply (terminal): %v", err)
	}
	if !interactiveReceived {
		t.Fatal("Apply: noInteractive was set when HasTerminal returns true; want interactive")
	}
}

func TestNoMachineCommandWhenRunning(t *testing.T) {
	var initCalled, startCalled bool
	d := deps.Deps{
		Out: newOut(io.Discard),
		MachineList: func(_ context.Context, _ string) ([]deps.MachineInfo, error) {
			return []deps.MachineInfo{{Name: "default", Running: true}}, nil
		},
		MachineInit: func(_ context.Context, _ string) error {
			initCalled = true
			return nil
		},
		MachineStart: func(_ context.Context, _ string) error {
			startCalled = true
			return nil
		},
		FindPodman: func(_ string) (string, error) {
			return "/usr/bin/podman", nil
		},
	}

	plan := deps.Plan{}
	if _, err := d.Apply(context.Background(), plan); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if initCalled {
		t.Fatal("Apply: machine init was called when machine is running")
	}
	if startCalled {
		t.Fatal("Apply: machine start was called when machine is running")
	}
}

func TestNoSecondMachine(t *testing.T) {
	var initCalled bool
	var startCalled bool
	d := deps.Deps{
		Out: newOut(io.Discard),
		MachineList: func(_ context.Context, _ string) ([]deps.MachineInfo, error) {
			return []deps.MachineInfo{{Name: "default", Running: false}}, nil
		},
		MachineInit: func(_ context.Context, _ string) error {
			initCalled = true
			return nil
		},
		MachineStart: func(_ context.Context, _ string) error {
			startCalled = true
			return nil
		},
		FindPodman: func(_ string) (string, error) {
			return "/usr/bin/podman", nil
		},
	}

	plan := deps.Plan{}
	if _, err := d.Apply(context.Background(), plan); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if initCalled {
		t.Fatal("Apply: machine init was called when machine already exists; want start only")
	}
	if !startCalled {
		t.Fatal("Apply: machine start was not called for stopped machine; want start")
	}
}
