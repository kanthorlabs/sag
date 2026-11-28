package deps

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/kanthorlabs/sag/internal/pins"
	"github.com/kanthorlabs/sag/internal/stepline"
)

func aptItem(version string) pins.Item {
	return pins.Item{
		Name: "podman", Version: version, Platform: "linux/*",
		Source: "apt", Locator: "podman", SHA256: "-",
	}
}

func TestAptVersionParse(t *testing.T) {
	v, err := aptVersionParse("1:5.4.2+ds1-2+b2")
	if err != nil {
		t.Fatalf("aptVersionParse: %v", err)
	}
	if v != "5.4.2" {
		t.Fatalf("aptVersionParse = %q; want %q", v, "5.4.2")
	}
}

func TestEpochRemoved(t *testing.T) {
	v, err := aptVersionParse("2:4.3.1-1")
	if err != nil {
		t.Fatalf("aptVersionParse: %v", err)
	}
	if v != "4.3.1" {
		t.Fatalf("aptVersionParse = %q; want %q", v, "4.3.1")
	}
}

func TestNoCandidateFailsInPlan(t *testing.T) {
	d := Deps{
		AptCandidateVersion: func(_ context.Context, _ string) (string, error) {
			return "", nil
		},
		IsAbsent: func() bool { return true },
		Out:      stepline.New(io.Discard),
	}
	_, err := d.Make(context.Background(), []pins.Item{aptItem("5.4.2")}, "", nil)
	var noCandidate *AptNoCandidateError
	if !errors.As(err, &noCandidate) {
		t.Fatalf("Make err = %v; want AptNoCandidateError", err)
	}
	if noCandidate.Name != "podman" {
		t.Fatalf("AptNoCandidateError.Name = %q; want %q", noCandidate.Name, "podman")
	}
}

func TestOlderCandidateFailsInPlan(t *testing.T) {
	d := Deps{
		AptCandidateVersion: func(_ context.Context, _ string) (string, error) {
			return "5.0.0", nil
		},
		IsAbsent: func() bool { return false },
		Out:      stepline.New(io.Discard),
	}
	_, err := d.Make(context.Background(), []pins.Item{aptItem("5.4.2")}, "/usr/bin/podman", nil)
	var olderErr *AptOlderCandidateError
	if !errors.As(err, &olderErr) {
		t.Fatalf("Make err = %v; want AptOlderCandidateError", err)
	}
	if olderErr.Candidate != "5.0.0" {
		t.Fatalf("AptOlderCandidateError.Candidate = %q; want %q", olderErr.Candidate, "5.0.0")
	}
	if olderErr.Pin != "5.4.2" {
		t.Fatalf("AptOlderCandidateError.Pin = %q; want %q", olderErr.Pin, "5.4.2")
	}
}

func TestNewuidmapMissingExit1(t *testing.T) {
	aptInstallCalled := false
	d := Deps{
		HasTerminal: func() bool { return false },
		AptInstall: func(_ context.Context, _ string, _ bool) error {
			aptInstallCalled = true
			return nil
		},
		FindNewuidmap: func() bool { return false },
		Out:           stepline.New(io.Discard),
	}
	plan := Plan{Items: []PlanItem{{
		Pin:    aptItem("5.4.2"),
		Absent: true,
		Approve: true,
	}}}
	_, err := d.Apply(context.Background(), plan)
	if !aptInstallCalled {
		t.Fatal("Apply: apt install was not called")
	}
	var newuidmapErr *NewuidmapMissingError
	if !errors.As(err, &newuidmapErr) {
		t.Fatalf("Apply err = %v; want NewuidmapMissingError", err)
	}
}

func TestAptSudoNonInteractive(t *testing.T) {
	var noInteractiveReceived bool
	d := Deps{
		HasTerminal: func() bool { return false },
		AptInstall: func(_ context.Context, _ string, noInteractive bool) error {
			noInteractiveReceived = noInteractive
			return nil
		},
		FindNewuidmap: func() bool { return true },
		FindPodman:    func(_ string) (string, error) { return "/usr/bin/podman", nil },
		Out:           stepline.New(io.Discard),
	}
	plan := Plan{Items: []PlanItem{{
		Pin:     aptItem("5.4.2"),
		Absent:  true,
		Approve: true,
	}}}
	if _, err := d.Apply(context.Background(), plan); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if !noInteractiveReceived {
		t.Fatal("Apply: noInteractive was not set to true when HasTerminal returns false")
	}

	var interactiveReceived bool
	d.HasTerminal = func() bool { return true }
	d.AptInstall = func(_ context.Context, _ string, noInteractive bool) error {
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
