package harness

import (
	"slices"
	"strings"
)

const SkipEnv = "SAG_AC_SKIP"

var sequentialRank = map[string]int{
	"AC16": 0,
	"AC08": 2,
	"AC19": 3,
}

const otherSequentialRank = 1

func effectivePhase(ac AC) Phase {
	if ac.Phase == PhaseSequential {
		return PhaseSequential
	}
	return PhaseParallel
}

func InPhase(acs []AC, phase Phase) []AC {
	var out []AC
	for _, ac := range acs {
		if effectivePhase(ac) == phase {
			out = append(out, ac)
		}
	}
	if phase == PhaseSequential {
		slices.SortStableFunc(out, func(a, b AC) int {
			return rank(a.ID) - rank(b.ID)
		})
	}
	return out
}

func rank(id string) int {
	if r, ok := sequentialRank[id]; ok {
		return r
	}
	return otherSequentialRank
}

func WithoutSkipped(acs []AC, skip string) []AC {
	skipped := strings.FieldsFunc(skip, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t'
	})
	var out []AC
	for _, ac := range acs {
		if !slices.Contains(skipped, ac.ID) {
			out = append(out, ac)
		}
	}
	return out
}
