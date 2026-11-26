package deps

import "testing"

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		a, b    string
		want    int
		wantErr bool
	}{
		{"5.0.0", "5.0.0", 0, false},
		{"5.1.0", "5.0.0", 1, false},
		{"4.9.3", "5.0.0", -1, false},
		{"5.10.0", "5.9.0", 1, false},
		{"5.5.1", "5.5.1", 0, false},
		{"5.0.0-rc1", "5.0.0", 0, false},
		{"5.0.0+build1", "5.0.0", 0, false},
		{"5.0.0~beta", "5.0.0", 0, false},
		{"4.9.3+ds1", "4.9.3", 0, false},
		{"5.0.0.alpha", "5.0.0", 0, true},
		{"5.alpha.0", "5.0.0", 0, true},
	}
	for _, tc := range tests {
		got, err := compareVersions(tc.a, tc.b)
		if tc.wantErr {
			if err == nil {
				t.Errorf("compareVersions(%q, %q) = %d, nil; want error", tc.a, tc.b, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("compareVersions(%q, %q): %v", tc.a, tc.b, err)
			continue
		}
		if got != tc.want {
			t.Errorf("compareVersions(%q, %q) = %d; want %d", tc.a, tc.b, got, tc.want)
		}
	}
}
