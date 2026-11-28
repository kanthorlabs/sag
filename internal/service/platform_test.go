package service

import "testing"

func TestCheckPlatform(t *testing.T) {
	for _, p := range [][2]string{{"darwin", "arm64"}, {"linux", "amd64"}, {"linux", "arm64"}} {
		if err := CheckPlatform(p[0], p[1]); err != nil {
			t.Fatalf("CheckPlatform(%s, %s) = %v; want nil", p[0], p[1], err)
		}
	}
	for _, p := range [][2]string{{"darwin", "amd64"}, {"windows", "amd64"}, {"linux", "386"}} {
		err := CheckPlatform(p[0], p[1])
		want := "unsupported platform " + p[0] + "/" + p[1]
		if err == nil || err.Error() != want {
			t.Fatalf("CheckPlatform(%s, %s) = %v; want %q", p[0], p[1], err, want)
		}
	}
}
