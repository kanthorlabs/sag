package bootstrap

import "context"

func (d Deps) stepSelectPins(_ context.Context, _ *State) error {
	return nil
}

func (d Deps) stepDeps(_ context.Context, _ *State) error {
	return nil
}

func defaultPlanDeps(findPodman func(stored string) (string, error)) func(context.Context, *State) error {
	return func(_ context.Context, s *State) error {
		if s.PodmanPath == "" {
			p, err := findPodman(s.Answers.PodmanPath)
			if err != nil {
				return err
			}
			s.PodmanPath = p
		}
		return nil
	}
}
