package main

import (
	"strings"
	"testing"
)

func TestTAPPassAllOk(t *testing.T) {
	tap := `TAP version 13
ok 1 - blocks sag credential delete
ok 2 - passes ls -la
1..2
`
	r := parseTAP(strings.NewReader(tap))
	if !tapPassed(r) {
		t.Errorf("parseTAP: total=%d failed=%d; want passed", r.Total, r.Failed)
	}
}

func TestTAPFailNotOk(t *testing.T) {
	tap := `TAP version 13
ok 1 - passes ls -la
not ok 2 - blocks sag credential delete
  ---
  error: Expected block but got undefined
  ...
1..2
`
	r := parseTAP(strings.NewReader(tap))
	if tapPassed(r) {
		t.Errorf("parseTAP: total=%d failed=%d; want not passed", r.Total, r.Failed)
	}
	if r.Failed != 1 {
		t.Errorf("failed=%d; want 1", r.Failed)
	}
}

func TestTAPEmptyFails(t *testing.T) {
	r := parseTAP(strings.NewReader(""))
	if tapPassed(r) {
		t.Errorf("parseTAP: empty input should not pass; total=%d", r.Total)
	}
}

func TestTAPNoTestsFails(t *testing.T) {
	tap := "TAP version 13\n1..0\n"
	r := parseTAP(strings.NewReader(tap))
	if tapPassed(r) {
		t.Errorf("parseTAP: zero tests should not pass; total=%d", r.Total)
	}
}

func TestTAPNonTAPLinesIgnored(t *testing.T) {
	tap := `TAP version 13
# some comment
ok 1 - first test
# another comment
  stderr output here
ok 2 - second test
1..2
`
	r := parseTAP(strings.NewReader(tap))
	if !tapPassed(r) {
		t.Errorf("parseTAP: non-TAP lines should be ignored; total=%d failed=%d", r.Total, r.Failed)
	}
	if r.Total != 2 {
		t.Errorf("total=%d; want 2", r.Total)
	}
}
