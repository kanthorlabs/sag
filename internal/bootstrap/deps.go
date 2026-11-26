package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"strings"

	deployfiles "github.com/kanthorlabs/sag/deploy"
	"github.com/kanthorlabs/sag/internal/deps"
	"github.com/kanthorlabs/sag/internal/pins"
	"github.com/kanthorlabs/sag/internal/prompt"
	"github.com/kanthorlabs/sag/internal/testhook"
)

func (d Deps) stepSelectPins(_ context.Context, s *State) error {
	content := testhook.Pins()
	if content == "" {
		content = string(deployfiles.Pins)
	}
	requireHTTPS := testhook.RequireHTTPS()
	var items []pins.Item
	for _, line := range strings.Split(content, "\n") {
		item, err := pins.ParseLine(line)
		if err != nil {
			var skipErr *pins.SkipError
			if errors.As(err, &skipErr) {
				continue
			}
			return fmt.Errorf("bootstrap: pins: %w", err)
		}
		if err := pins.ValidateItem(item, requireHTTPS); err != nil {
			return fmt.Errorf("bootstrap: %w", err)
		}
		items = append(items, item)
	}
	s.Pins = pins.SelectPlatform(items, d.GOOS(), d.GOARCH())
	if len(s.Pins) == 0 {
		return fmt.Errorf("bootstrap: no pins for platform %s/%s", d.GOOS(), d.GOARCH())
	}
	return nil
}

func (d Deps) stepDeps(ctx context.Context, s *State) error {
	if len(s.DepPlan.Items) == 0 {
		return nil
	}

	dd := deps.Deps{
		PodmanVersion:     deps.DefaultPodmanVersion,
		Download:          deps.DefaultDownload,
		SHA256:            deps.DefaultSHA256,
		Install:           deps.DefaultInstall,
		FindPodman:        d.FindPodman,
		MachineList:       deps.DefaultMachineList,
		MachineInit:       deps.DefaultMachineInit,
		MachineStart:      deps.DefaultMachineStart,
		HasTerminal:       makeHasTerminal(s.Asker),
		AskInstall:        makeAskInstall(s.Asker),
		IsAbsent:          testhook.Absent,
		StopBeforeInstall: testhook.StopBeforeInstall,
		Out:               s.Out,
		Print:             d.Print,
	}

	newPath, err := dd.Apply(ctx, s.DepPlan)
	if err != nil {
		return err
	}
	if newPath != "" && newPath != s.PodmanPath {
		s.PodmanPath = newPath
		merged := s.Answers
		merged.PodmanPath = newPath
		if err := d.SaveAnswers(s.Paths, merged); err != nil {
			return err
		}
		s.Answers = merged
	}
	return nil
}

func defaultPlanDeps(findPodman func(stored string) (string, error)) func(context.Context, *State) error {
	return func(ctx context.Context, s *State) error {
		if s.PodmanPath == "" {
			p, err := findPodman(s.Answers.PodmanPath)
			if err == nil {
				s.PodmanPath = p
			}
		}

		if len(s.Pins) == 0 {
			return nil
		}

		if len(s.DepPlan.Items) > 0 {
			return nil
		}

		dd := deps.Deps{
			PodmanVersion: deps.DefaultPodmanVersion,
			IsAbsent:      testhook.Absent,
			Out:           s.Out,
			HasTerminal:   makeHasTerminal(s.Asker),
			AskInstall:    makeAskInstall(s.Asker),
		}

		plan, err := dd.Make(ctx, s.Pins, s.PodmanPath, s.Flags.AutoApprove)
		if err != nil {
			return err
		}
		s.DepPlan = plan
		return nil
	}
}

func makeHasTerminal(asker prompt.Asker) func() bool {
	return func() bool {
		tty, err := asker.Open()
		if err != nil {
			return false
		}
		tty.Close()
		return true
	}
}

func makeAskInstall(asker prompt.Asker) func(name, version, locator string) (bool, error) {
	return func(name, version, locator string) (bool, error) {
		return asker.YesNo("Install " + name + " " + version + " from " + locator + "? [y/N] ")
	}
}
