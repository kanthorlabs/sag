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
	IsAbsent          func() bool
	StopBeforeInstall func() bool
	Out               stepline.Writer
	Print             io.Writer
}

func (d Deps) Make(ctx context.Context, items []pins.Item, podmanPath string, autoApprove []string) (Plan, error) {
	var plan Plan
	for _, item := range items {
		if item.Source != "release" {
			continue
		}

		pi := PlanItem{Pin: item}

		absent := d.IsAbsent != nil && d.IsAbsent()
		if !absent && podmanPath == "" {
			absent = true
		}

		if !absent {
			version, err := d.PodmanVersion(ctx, podmanPath)
			if err != nil {
				return Plan{}, fmt.Errorf("deps: %s --version: %w", item.Name, err)
			}
			cmp, err := compareVersions(version, item.Version)
			if err != nil {
				return Plan{}, fmt.Errorf("deps: version parse: %w", err)
			}
			switch {
			case cmp < 0:
				d.Out.Line(stepline.StatusFail, item.Name, "installed "+version+", pin "+item.Version)
				return Plan{}, &OlderError{Name: item.Name, Installed: version, Pin: item.Version}
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
						return Plan{}, err
					}
					if !ok {
						d.Out.Line(stepline.StatusFail, item.Name, "absent; run on a terminal or add --auto-approve "+item.Name+"@"+item.Version)
						return Plan{}, &InstallRefusal{Name: item.Name, Version: item.Version}
					}
					approved = true
				} else {
					d.Out.Line(stepline.StatusFail, item.Name, "absent; run on a terminal or add --auto-approve "+item.Name+"@"+item.Version)
					return Plan{}, &InstallRefusal{Name: item.Name, Version: item.Version}
				}
			}
			pi.Approve = approved
		}

		plan.Items = append(plan.Items, pi)
	}
	return plan, nil
}

func (d Deps) Apply(ctx context.Context, plan Plan) (string, error) {
	var newPodmanPath string
	for _, pi := range plan.Items {
		if !pi.Absent || !pi.Approve {
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
			if err == nil {
				newPodmanPath = p
			}
		}

		if newPodmanPath != "" && d.PodmanVersion != nil {
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
	n := len(aParts)
	if len(bParts) > n {
		n = len(bParts)
	}
	for i := 0; i < n; i++ {
		var av, bv int
		if i < len(aParts) {
			v, err := strconv.Atoi(aParts[i])
			if err != nil {
				return 0, fmt.Errorf("deps: invalid version part %q in %q", aParts[i], a)
			}
			av = v
		}
		if i < len(bParts) {
			v, err := strconv.Atoi(bParts[i])
			if err != nil {
				return 0, fmt.Errorf("deps: invalid version part %q in %q", bParts[i], b)
			}
			bv = v
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
