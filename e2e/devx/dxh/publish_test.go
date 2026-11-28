package dxh

import "testing"

func TestParseRouteSrc(t *testing.T) {
	got, err := parseRouteSrc("1.1.1.1 via 10.0.0.1 dev eth0 src 10.0.0.7 uid 1000 \n    cache \n")
	if err != nil || got != "10.0.0.7" {
		t.Fatalf("parseRouteSrc = %q, %v; want 10.0.0.7", got, err)
	}
	for _, in := range []string{"", "1.1.1.1 dev lo src 127.0.0.1", "1.1.1.1 dev eth0 src fe80::1", "1.1.1.1 dev eth0 src"} {
		if got, err := parseRouteSrc(in); err == nil {
			t.Fatalf("parseRouteSrc(%q) = %q; want error", in, got)
		}
	}
}
