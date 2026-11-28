package deps

import (
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/kanthorlabs/sag/internal/pins"
	"github.com/kanthorlabs/sag/internal/stepline"
)

type Plan struct {
	Items []PlanItem
}

type PlanItem struct {
	Pin     pins.Item
	Absent  bool
	Older   bool
	Newer   bool
	Approve bool
}

type MachineInfo struct {
	Name    string
	Running bool
}

type InstallRefusal struct {
	Name    string
	Version string
}

func (e *InstallRefusal) Error() string {
	return fmt.Sprintf("install of %s requires approval; add --auto-approve %s@%s", e.Name, e.Name, e.Version)
}

type SudoRefusal struct{}

func (e *SudoRefusal) Error() string {
	return "sudo -n refused installer access; run on a terminal to authorize"
}

type OlderError struct {
	Name      string
	Installed string
	Pin       string
}

func (e *OlderError) Error() string {
	return fmt.Sprintf("deps: %s installed %s is older than pin %s", e.Name, e.Installed, e.Pin)
}

type AptNoCandidateError struct {
	Name string
}

func (e *AptNoCandidateError) Error() string {
	return fmt.Sprintf("deps: apt: no candidate for %s", e.Name)
}

type AptOlderCandidateError struct {
	Name      string
	Candidate string
	Pin       string
}

func (e *AptOlderCandidateError) Error() string {
	return fmt.Sprintf("deps: apt: %s candidate %s is older than pin %s", e.Name, e.Candidate, e.Pin)
}

type NewuidmapMissingError struct {
	Name string
}

func (e *NewuidmapMissingError) Error() string {
	return fmt.Sprintf("deps: %s: newuidmap not found in PATH", e.Name)
}

type Deps struct {
	PodmanVersion     func(ctx context.Context, path string) (string, error)
	Download          func(ctx context.Context, url, dir string) (string, error)
	SHA256            func(path string) (string, error)
	Install           func(ctx context.Context, pkgPath string, noInteractive bool) error
	FindPodman        func(stored string) (string, error)
	MachineList       func(ctx context.Context, path string) ([]MachineInfo, error)
	MachineInit       func(ctx context.Context, path string) error
	MachineStart      func(ctx context.Context, path string) error
	HasTerminal       func() bool
	AskInstall        func(name, version, locator string) (bool, error)
	AptCandidateVersion func(ctx context.Context, name string) (string, error)
	AptInstall          func(ctx context.Context, name string, noInteractive bool) error
	FindNewuidmap       func() bool
	AskAptInstall       func(name, candidate, pin string) (bool, error)
	IsAbsent          func() bool
	StopBeforeInstall func() bool
	Out               stepline.Writer
	Print             io.Writer
}

func (d Deps) Make(ctx context.Context, items []pins.Item, podmanPath string, autoApprove []string) (Plan, error) {
	var plan Plan
	for _, item := range items {
		switch item.Source {
		case "release":
			pi, err := d.makeReleaseItem(ctx, item, podmanPath, autoApprove)
			if err != nil {
				return Plan{}, err
			}
			plan.Items = append(plan.Items, pi)
		case "apt":
			if d.AptCandidateVersion == nil {
				continue
			}
			pi, err := d.makeAptItem(ctx, item, podmanPath, autoApprove)
			if err != nil {
				return Plan{}, err
			}
			plan.Items = append(plan.Items, pi)
		}
	}
	return plan, nil
}

func (d Deps) makeReleaseItem(ctx context.Context, item pins.Item, podmanPath string, autoApprove []string) (PlanItem, error) {
	pi := PlanItem{Pin: item}

	absent := d.IsAbsent != nil && d.IsAbsent()
	if !absent && podmanPath == "" {
		absent = true
	}

	if !absent {
		version, err := d.PodmanVersion(ctx, podmanPath)
		if err != nil {
			return PlanItem{}, fmt.Errorf("deps: %s --version: %w", item.Name, err)
		}
		cmp, err := compareVersions(version, item.Version)
		if err != nil {
			return PlanItem{}, fmt.Errorf("deps: version parse: %w", err)
		}
		switch {
		case cmp < 0:
			d.Out.Line(stepline.StatusFail, item.Name, "installed "+version+", pin "+item.Version)
			return PlanItem{}, &OlderError{Name: item.Name, Installed: version, Pin: item.Version}
		case cmp == 0:
			d.Out.Line(stepline.StatusOK, item.Name, version)
		case cmp > 0:
			pi.Newer = true
			d.Out.Line(stepline.StatusWarn, item.Name, "installed "+version+", pin "+item.Version)
		}
	}

	if absent {
		pi.Absent = true
		approved, warnMsg := checkAutoApprove(autoApprove, item)
		if warnMsg != "" {
			d.Out.Line(stepline.StatusWarn, item.Name, warnMsg)
		}
		if !approved {
			if d.HasTerminal != nil && d.HasTerminal() {
				ok, err := d.AskInstall(item.Name, item.Version, item.Locator)
				if err != nil {
					return PlanItem{}, err
				}
				if !ok {
					d.Out.Line(stepline.StatusFail, item.Name, "absent; run on a terminal or add --auto-approve "+item.Name+"@"+item.Version)
					return PlanItem{}, &InstallRefusal{Name: item.Name, Version: item.Version}
				}
				approved = true
			} else {
				d.Out.Line(stepline.StatusFail, item.Name, "absent; run on a terminal or add --auto-approve "+item.Name+"@"+item.Version)
				return PlanItem{}, &InstallRefusal{Name: item.Name, Version: item.Version}
			}
		}
		pi.Approve = approved
	}

	return pi, nil
}

func (d Deps) makeAptItem(ctx context.Context, item pins.Item, podmanPath string, autoApprove []string) (PlanItem, error) {
	pi := PlanItem{Pin: item}

	candidate, err := d.AptCandidateVersion(ctx, item.Locator)
	if err != nil {
		return PlanItem{}, fmt.Errorf("deps: apt: %s: %w", item.Name, err)
	}
	if candidate == "" {
		d.Out.Line(stepline.StatusFail, item.Name, "no apt candidate")
		return PlanItem{}, &AptNoCandidateError{Name: item.Name}
	}
	cmp, err := compareVersions(candidate, item.Version)
	if err != nil {
		return PlanItem{}, fmt.Errorf("deps: apt version parse: %w", err)
	}
	if cmp < 0 {
		d.Out.Line(stepline.StatusFail, item.Name, "candidate "+candidate+", pin "+item.Version)
		return PlanItem{}, &AptOlderCandidateError{Name: item.Name, Candidate: candidate, Pin: item.Version}
	}

	absent := d.IsAbsent != nil && d.IsAbsent()
	if !absent && podmanPath == "" {
		absent = true
	}

	if absent {
		pi.Absent = true
		approved, warnMsg := checkAutoApprove(autoApprove, item)
		if warnMsg != "" {
			d.Out.Line(stepline.StatusWarn, item.Name, warnMsg)
		}
		if !approved {
			if d.HasTerminal != nil && d.HasTerminal() {
				var ok bool
				if d.AskAptInstall != nil {
					ok, err = d.AskAptInstall(item.Name, candidate, item.Version)
				} else {
					ok, err = d.AskInstall(item.Name, item.Version, item.Locator)
				}
				if err != nil {
					return PlanItem{}, err
				}
				if !ok {
					d.Out.Line(stepline.StatusFail, item.Name, "absent; run on a terminal or add --auto-approve "+item.Name+"@"+item.Version)
					return PlanItem{}, &InstallRefusal{Name: item.Name, Version: item.Version}
				}
				approved = true
			} else {
				d.Out.Line(stepline.StatusFail, item.Name, "absent; run on a terminal or add --auto-approve "+item.Name+"@"+item.Version)
				return PlanItem{}, &InstallRefusal{Name: item.Name, Version: item.Version}
			}
		}
		pi.Approve = approved
	} else {
		if cmp == 0 {
			d.Out.Line(stepline.StatusOK, item.Name, candidate)
		} else {
			pi.Newer = true
			d.Out.Line(stepline.StatusWarn, item.Name, "candidate "+candidate+", pin "+item.Version)
		}
	}

	return pi, nil
}

func (d Deps) Apply(ctx context.Context, plan Plan) (string, error) {
	var newPodmanPath string
	for _, pi := range plan.Items {
		if !pi.Absent || !pi.Approve {
			continue
		}

		if pi.Pin.Source == "apt" {
			p, err := d.applyAptItem(ctx, pi)
			if err != nil {
				return newPodmanPath, err
			}
			if p != "" {
				newPodmanPath = p
			}
			continue
		}

		tmpDir, err := os.MkdirTemp("", "sag-deps-*")
		if err != nil {
			return "", fmt.Errorf("deps: mktemp: %w", err)
		}
		defer os.RemoveAll(tmpDir)

		pkgPath, err := d.Download(ctx, pi.Pin.Locator, tmpDir)
		if err != nil {
			return "", fmt.Errorf("deps: download %s: %w", pi.Pin.Name, err)
		}

		got, err := d.SHA256(pkgPath)
		if err != nil {
			os.Remove(pkgPath)
			return "", fmt.Errorf("deps: sha256 %s: %w", pi.Pin.Name, err)
		}
		if !strings.EqualFold(got, pi.Pin.SHA256) {
			os.Remove(pkgPath)
			return "", fmt.Errorf("deps: checksum mismatch for %s: got %s, want %s", pi.Pin.Name, got, pi.Pin.SHA256)
		}

		d.Out.Line(stepline.StatusInstall, pi.Pin.Name, pi.Pin.Version)

		if d.StopBeforeInstall != nil && d.StopBeforeInstall() {
			if d.Print != nil {
				fmt.Fprintln(d.Print, pkgPath)
			}
			continue
		}

		noInteractive := d.HasTerminal == nil || !d.HasTerminal()
		if err := d.Install(ctx, pkgPath, noInteractive); err != nil {
			return "", fmt.Errorf("deps: install %s: %w", pi.Pin.Name, err)
		}

		if d.FindPodman != nil {
			p, err := d.FindPodman("")
			if err != nil {
				return newPodmanPath, fmt.Errorf("deps: find %s after install: %w", pi.Pin.Name, err)
			}
			newPodmanPath = p
		}

		if newPodmanPath != "" {
			ver, verErr := d.PodmanVersion(ctx, newPodmanPath)
			if verErr != nil {
				return newPodmanPath, fmt.Errorf("deps: %s version after install: %w", pi.Pin.Name, verErr)
			}
			cmp, cmpErr := compareVersions(ver, pi.Pin.Version)
			if cmpErr != nil || cmp < 0 {
				return newPodmanPath, fmt.Errorf("deps: installed %s %s is older than pin %s", pi.Pin.Name, ver, pi.Pin.Version)
			}
		}
	}

	podmanPath := newPodmanPath
	if podmanPath == "" && d.FindPodman != nil {
		p, err := d.FindPodman("")
		if err == nil {
			podmanPath = p
		}
	}

	if podmanPath != "" && d.MachineList != nil {
		machines, err := d.MachineList(ctx, podmanPath)
		if err != nil {
			return newPodmanPath, fmt.Errorf("deps: machine list: %w", err)
		}
		if len(machines) == 0 {
			if err := d.MachineInit(ctx, podmanPath); err != nil {
				return newPodmanPath, fmt.Errorf("deps: machine init: %w", err)
			}
			if err := d.MachineStart(ctx, podmanPath); err != nil {
				return newPodmanPath, fmt.Errorf("deps: machine start: %w", err)
			}
		} else {
			m := machines[0]
			if !m.Running {
				if err := d.MachineStart(ctx, podmanPath); err != nil {
					return newPodmanPath, fmt.Errorf("deps: machine start: %w", err)
				}
			}
		}
	}

	return newPodmanPath, nil
}

func (d Deps) applyAptItem(ctx context.Context, pi PlanItem) (string, error) {
	noInteractive := d.HasTerminal == nil || !d.HasTerminal()
	if err := d.AptInstall(ctx, pi.Pin.Locator, noInteractive); err != nil {
		return "", fmt.Errorf("deps: apt install %s: %w", pi.Pin.Name, err)
	}
	if d.FindNewuidmap != nil && !d.FindNewuidmap() {
		d.Out.Line(stepline.StatusFail, pi.Pin.Name, "newuidmap not found")
		return "", &NewuidmapMissingError{Name: pi.Pin.Name}
	}
	d.Out.Line(stepline.StatusInstall, pi.Pin.Name, pi.Pin.Version)
	if d.FindPodman == nil {
		return "", nil
	}
	p, err := d.FindPodman("")
	if err != nil {
		return "", fmt.Errorf("deps: find %s after apt install: %w", pi.Pin.Name, err)
	}
	return p, nil
}

func stripVersionSuffix(v string) string {
	if i := strings.IndexAny(v, "-+~"); i >= 0 {
		return v[:i]
	}
	return v
}

func compareVersions(a, b string) (int, error) {
	a = stripVersionSuffix(a)
	b = stripVersionSuffix(b)
	aParts := strings.Split(a, ".")
	bParts := strings.Split(b, ".")

	aInts := make([]int, len(aParts))
	for i, p := range aParts {
		v, err := strconv.Atoi(p)
		if err != nil {
			return 0, fmt.Errorf("deps: invalid version part %q in %q", p, a)
		}
		aInts[i] = v
	}
	bInts := make([]int, len(bParts))
	for i, p := range bParts {
		v, err := strconv.Atoi(p)
		if err != nil {
			return 0, fmt.Errorf("deps: invalid version part %q in %q", p, b)
		}
		bInts[i] = v
	}

	n := len(aInts)
	if len(bInts) > n {
		n = len(bInts)
	}
	for i := 0; i < n; i++ {
		av, bv := 0, 0
		if i < len(aInts) {
			av = aInts[i]
		}
		if i < len(bInts) {
			bv = bInts[i]
		}
		if av < bv {
			return -1, nil
		}
		if av > bv {
			return 1, nil
		}
	}
	return 0, nil
}

func checkAutoApprove(autoApprove []string, item pins.Item) (approved bool, warnMsg string) {
	target := item.Name + "@" + item.Version
	for _, aa := range autoApprove {
		if aa == target {
			return true, ""
		}
	}
	for _, aa := range autoApprove {
		name, ver, _ := strings.Cut(aa, "@")
		if name == item.Name {
			return false, "pre-approval " + ver + " does not match pin " + item.Version + "; ignored"
		}
	}
	return false, ""
}
