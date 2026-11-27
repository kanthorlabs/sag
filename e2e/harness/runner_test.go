package harness

import (
	"reflect"
	"testing"
)

func ids(acs []AC) []string {
	out := make([]string, len(acs))
	for i, ac := range acs {
		out[i] = ac.ID
	}
	return out
}

func TestPhaseOrder(t *testing.T) {
	acs := []AC{
		{ID: "AC19", Phase: PhaseSequential},
		{ID: "AC05", Phase: PhaseParallel},
		{ID: "AC08", Phase: PhaseSequential},
		{ID: "AC50", Phase: PhaseSequential},
		{ID: "AC16", Phase: PhaseSequential},
		{ID: "Harness_Skeleton", Phase: PhaseParallel},
	}
	if got, want := ids(InPhase(acs, PhaseParallel)), []string{"AC05", "Harness_Skeleton"}; !reflect.DeepEqual(got, want) {
		t.Errorf("parallel = %v, want %v", got, want)
	}
	if got, want := ids(InPhase(acs, PhaseSequential)), []string{"AC16", "AC50", "AC08", "AC19"}; !reflect.DeepEqual(got, want) {
		t.Errorf("sequential = %v, want %v", got, want)
	}
}

func TestPhaseZeroIsParallel(t *testing.T) {
	acs := []AC{{ID: "AC01"}}
	if got := ids(InPhase(acs, PhaseParallel)); !reflect.DeepEqual(got, []string{"AC01"}) {
		t.Errorf("parallel = %v, want [AC01]", got)
	}
}

func TestWithoutSkipped(t *testing.T) {
	acs := []AC{{ID: "AC01"}, {ID: "AC31"}, {ID: "AC33"}, {ID: "AC35"}}
	got := ids(WithoutSkipped(acs, " AC31, AC35 "))
	if want := []string{"AC01", "AC33"}; !reflect.DeepEqual(got, want) {
		t.Errorf("WithoutSkipped = %v, want %v", got, want)
	}
	if got := ids(WithoutSkipped(acs, "")); len(got) != 4 {
		t.Errorf("empty skip list removed ACs: %v", got)
	}
}

func TestParseToken(t *testing.T) {
	jwt := "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiJhYzkwIn0.c2lnbmF0dXJlLXZhbHVl_-"
	got, err := parseToken("\x1b[?2004h" + jwt + "\r\n")
	if err != nil || got != jwt {
		t.Errorf("parseToken = %q, %v; want %q", got, err, jwt)
	}
	if _, err := parseToken("error: no terminal\r\n"); err == nil {
		t.Error("parseToken accepted output without a JWT")
	}
}
