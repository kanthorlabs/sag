package testenv

import "testing"

func TestMajorFromEnv(t *testing.T) {
	cases := []struct {
		value string
		want  int
		ok    bool
	}{
		{"", DefaultMajor, true},
		{"14", 14, true},
		{"19", 19, true},
		{"13", 0, false},
		{"20", 0, false},
		{"abc", 0, false},
	}
	for _, c := range cases {
		t.Run(c.value, func(t *testing.T) {
			t.Setenv(MajorEnv, c.value)
			got, err := MajorFromEnv()
			if c.ok && (err != nil || got != c.want) {
				t.Fatalf("got %d, %v; want %d", got, err, c.want)
			}
			if !c.ok && err == nil {
				t.Fatalf("got %d; want error", got)
			}
		})
	}
}

func TestDSNsHaveNoQueryParams(t *testing.T) {
	p := &Postgres{Major: 17, Container: "sag-e2e-abc-pg17", HostPort: 54321, password: "pw"}
	if got, want := p.HostDSN(), "postgres://postgres:pw@localhost:54321/postgres"; got != want {
		t.Errorf("HostDSN = %q, want %q", got, want)
	}
	if got, want := p.NetworkDSN(), "postgres://postgres:pw@sag-e2e-abc-pg17:5432/postgres"; got != want {
		t.Errorf("NetworkDSN = %q, want %q", got, want)
	}
}
