package harness

import (
	"strings"
	"testing"
)

func TestKeepSafePGLinesKeepsConnectionLines(t *testing.T) {
	in := strings.Join([]string{
		"2026-11-27 04:39:14 UTC [60] LOG:  connection received: host=10.89.0.3 port=54321",
		"2026-11-27 04:39:14 UTC [60] LOG:  connection authorized: user=postgres database=postgres",
		"2026-11-27 04:39:14 UTC [61] LOG:  statement: ALTER ROLE r1 WITH LOGIN PASSWORD 'k3y'",
		"2026-11-27 04:39:16 UTC [60] LOG:  disconnection: session time: 0:00:00.012 user=postgres database=postgres",
		"",
	}, "\n")
	got := keepSafePGLines(in)
	for _, keep := range []string{"connection received", "connection authorized", "disconnection"} {
		if !strings.Contains(got, keep) {
			t.Errorf("output lost %q:\n%s", keep, got)
		}
	}
	for _, drop := range []string{"ALTER ROLE", "k3y"} {
		if strings.Contains(got, drop) {
			t.Errorf("output kept %q:\n%s", drop, got)
		}
	}
}

func TestKeepSafePGLinesDropsStatements(t *testing.T) {
	in := strings.Join([]string{
		"2026-11-27 04:39:14 UTC [61] LOG:  checkpoint starting: time",
		"2026-11-27 04:39:15 UTC [70] LOG:  statement: ALTER ROLE r1 WITH LOGIN",
		"\tPASSWORD 'k3y' VALID UNTIL 'infinity'",
		"2026-11-27 04:39:16 UTC [71] ERROR:  cannot execute INSERT in a read-only transaction",
		"2026-11-27 04:39:16 UTC [71] STATEMENT:  SELECT f_insert()",
		"2026-11-27 04:39:17 UTC [72] FATAL:  sorry, too many clients already",
		"",
	}, "\n")
	got := keepSafePGLines(in)
	for _, keep := range []string{"checkpoint starting", "read-only transaction", "too many clients"} {
		if !strings.Contains(got, keep) {
			t.Errorf("output lost %q:\n%s", keep, got)
		}
	}
	for _, drop := range []string{"ALTER ROLE", "k3y", "f_insert"} {
		if strings.Contains(got, drop) {
			t.Errorf("output kept %q:\n%s", drop, got)
		}
	}
}
